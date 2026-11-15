package guardrule

import (
	"slices"
	"strings"
)

// The option reader (spec 0062, "Options are read by one module inside
// guardrule"). A program's options are read in one place, from a table
// that says how the program reads them, so that the guard finds where a
// wrapper's command begins, and which operands a writer writes, as the
// program does. A long option whose argument kind the table cannot tell
// gives two readings: one with the next word as its argument, one
// without.

// argKind is how an option takes an argument.
type argKind int

const (
	// argNone: the option takes no argument.
	argNone argKind = iota
	// argRequired: the rest of the word (-ufoo, --user=foo), or else the
	// next word.
	argRequired
	// argOptional: only an attached argument (sed -i.bak, --opt=value).
	argOptional
	// argUnknown: the table cannot tell; the option is read both ways.
	argUnknown
)

// optionTable says how a program reads its options.
type optionTable struct {
	// shortArg lists the short options that take an argument; shortOptArg
	// those whose argument is optional and only attached. Any other short
	// option takes none.
	shortArg, shortOptArg string
	// long maps each long option, without its "--", to its argument kind.
	long map[string]argKind
	// abbrev: a long option may be written as a prefix of its name, as
	// getopt_long reads it. An exact match wins; otherwise the prefix
	// stands for every long option it begins.
	abbrev bool
	// minAbbrev is the shortest prefix, in letters after "--", that is
	// read as an abbreviation.
	minAbbrev int
	// unknownLong is the kind of a long option that is not in long, or
	// not a long enough prefix of one: argUnknown for a table that lists
	// every option, argNone for one that lists only the options that take
	// an argument.
	unknownLong argKind
	// ends lists options, by their option.name, after which the program
	// reads no more options ("S" and "--split-string" for env): the words
	// after the word that holds one are operands. Only the last option of
	// a word is looked up.
	ends []string
	// permute: options may follow operands, as GNU getopt reads them by
	// default. Without it the first operand ends the options, as POSIX
	// getopt and getopt with a leading "+" read them.
	permute bool
	// plus: a word that begins with "+" is a cluster of short options too
	// (a shell's +o, +O, +e), read like one that begins with "-". Its
	// options have the same names.
	plus bool
	// nextWordArg: a short option that takes an argument always takes the
	// next word, and the rest of its cluster is read as options, as a
	// POSIX shell reads -o and -O (bash -oc posix 'ls' is -o posix -c).
	nextWordArg bool
}

// option is one option as it was read.
type option struct {
	// name is a short option's letter ("u"), or a long option's name with
	// its "--": the full name when the table resolves it to one option,
	// else as written ("--user", "--newopt").
	name string
	// value is the option's argument; hasValue is false when it has none.
	value    Word
	hasValue bool
}

// optionReading is one way to read a command's arguments.
type optionReading struct {
	opts []option
	// operands are the words that are not options. Without permute they
	// are the first operand and every word after it.
	operands []Word
}

// has reports whether the reading holds one of the options names.
func (r optionReading) has(names ...string) bool {
	return slices.ContainsFunc(r.opts, func(o option) bool { return slices.Contains(names, o.name) })
}

// maxReadings bounds how many readings one argument list gives. A list
// with more is not read: readOptions reports false, and the caller fails
// closed.
const maxReadings = 64

// readOptions reads args (without the program name) by t. It returns
// every reading, in a stable order: the one that reads each ambiguous
// long option without an argument first. ok is false when there are
// more than maxReadings.
func readOptions(args []Word, t optionTable) (readings []optionReading, ok bool) {
	pending := []readState{{}}
	for len(pending) > 0 {
		s := pending[0]
		pending = pending[1:]
		r, forks := t.read(args, s)
		readings = append(readings, r)
		pending = append(pending, forks...)
		if len(readings)+len(pending) > maxReadings {
			return nil, false
		}
	}
	return readings, true
}

// readState is a reading in progress: the next word to read and what is
// read so far.
type readState struct {
	i int
	optionReading
}

// read reads args from s to the end, and returns the reading with the
// readings it set aside at each ambiguous long option.
func (t optionTable) read(args []Word, s readState) (optionReading, []readState) {
	var forks []readState
	for s.i < len(args) {
		w := args[s.i]
		switch {
		case !w.Literal || w.Text == "-" || !strings.HasPrefix(w.Text, "-") && !t.isPlusCluster(w.Text):
			if !t.permute {
				s.operands = append(s.operands, args[s.i:]...)
				return s.optionReading, forks
			}
			s.operands = append(s.operands, w)
			s.i++
			continue
		case w.Text == "--":
			s.operands = append(s.operands, args[s.i+1:]...)
			return s.optionReading, forks
		case strings.HasPrefix(w.Text, "--"):
			if fork, ok := t.readLong(args, &s); ok {
				forks = append(forks, fork)
			}
		default:
			t.readShort(args, &s)
		}
		if n := len(s.opts); n > 0 && slices.Contains(t.ends, s.opts[n-1].name) {
			s.operands = append(s.operands, args[s.i:]...)
			return s.optionReading, forks
		}
	}
	return s.optionReading, forks
}

// add records option o and moves past the words it took.
func (s *readState) add(o option, words int) {
	s.opts = append(s.opts, o)
	s.i += words
}

// readLong reads the long option at s.i. When its argument kind is
// unknown, it reads it without an argument and returns the reading that
// takes the next word as its argument.
func (t optionTable) readLong(args []Word, s *readState) (readState, bool) {
	written, attached, isAttached := strings.Cut(args[s.i].Text[2:], "=")
	full, kind := t.longKind(written)
	o := option{name: "--" + full}
	hasNext := s.i+1 < len(args)
	switch {
	case isAttached:
		o.value, o.hasValue = literalWord(attached), true
	case kind == argRequired && hasNext:
		o.value, o.hasValue = args[s.i+1], true
		s.add(o, 2)
		return readState{}, false
	case kind == argUnknown && hasNext:
		fork := readState{i: s.i + 2, optionReading: optionReading{
			opts:     append(slices.Clone(s.opts), option{name: o.name, value: args[s.i+1], hasValue: true}),
			operands: slices.Clone(s.operands),
		}}
		s.add(o, 1)
		return fork, true
	}
	s.add(o, 1)
	return readState{}, false
}

// readShort reads the cluster of short options at s.i (-xvf file). The
// first option in it that takes an argument takes the rest of the
// cluster, or else the next word when its argument is required.
func (t optionTable) readShort(args []Word, s *readState) {
	if t.nextWordArg {
		t.readShortNextWord(args, s)
		return
	}
	text := args[s.i].Text
	for j := 1; j < len(text); j++ {
		o := option{name: text[j : j+1]}
		rest := text[j+1:]
		takesArg := strings.Contains(t.shortArg, o.name)
		switch {
		case (takesArg || strings.Contains(t.shortOptArg, o.name)) && rest != "":
			o.value, o.hasValue = literalWord(rest), true
		case takesArg && s.i+1 < len(args):
			o.value, o.hasValue = args[s.i+1], true
			s.add(o, 2)
			return
		case takesArg || strings.Contains(t.shortOptArg, o.name):
		default:
			s.opts = append(s.opts, o)
			continue
		}
		s.add(o, 1)
		return
	}
	s.i++
}

// readShortNextWord reads the cluster of short options at s.i as a POSIX
// shell does: each option in it that takes an argument takes the next
// word not yet taken, and the cluster goes on.
func (t optionTable) readShortNextWord(args []Word, s *readState) {
	next := s.i + 1
	for _, r := range args[s.i].Text[1:] {
		o := option{name: string(r)}
		if strings.ContainsRune(t.shortArg, r) && next < len(args) {
			o.value, o.hasValue = args[next], true
			next++
		}
		s.opts = append(s.opts, o)
	}
	s.i = next
}

// isPlusCluster reports whether text is a cluster of short options that
// begins with "+", for a table that reads them.
func (t optionTable) isPlusCluster(text string) bool {
	return t.plus && len(text) > 1 && text[0] == '+'
}

func literalWord(text string) Word { return Word{Text: text, Literal: true} }

// longKind resolves a long option written as name (without "--") and
// returns its full name, or name itself when it stands for no single
// option, with its argument kind. A prefix of several options whose
// kinds agree on whether they take the next word reads as that kind.
func (t optionTable) longKind(name string) (string, argKind) {
	if k, ok := t.long[name]; ok {
		return name, k
	}
	if !t.abbrev || len(name) < max(t.minAbbrev, 1) {
		return name, t.unknownLong
	}
	var full string
	required, other := false, false
	n := 0
	for l, k := range t.long {
		if !strings.HasPrefix(l, name) {
			continue
		}
		n++
		full = l
		if k == argRequired {
			required = true
		} else {
			other = true
		}
	}
	switch {
	case n == 0:
		return name, t.unknownLong
	case n == 1:
		return full, t.long[full]
	case required && other:
		return name, argUnknown
	case required:
		return name, argRequired
	}
	return name, argOptional
}
