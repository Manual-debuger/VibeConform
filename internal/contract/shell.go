package contract

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// Shell parsing for the "called" checks (spec 0051, "Rules"; spec 0052,
// "What static checks read"). Hook commands, CI run: steps, entrypoints
// and husky scripts are parsed as POSIX shell with mvdan.cc/sh (ADR
// 0026).

// Word is one shell word after quote removal.
type Word struct {
	// Text is the word after quote removal. Each expansion (parameter,
	// command substitution, arithmetic, ...) is replaced by expansionMark,
	// so a word with an expansion never ends in a literal suffix it does
	// not have.
	Text string
	// Literal is false when the word holds an expansion. Such a word
	// equals no other word: its value is only known when it runs.
	Literal bool
}

// expansionMark stands for an expansion inside Word.Text. It is the
// guard's mark, so a word reads the same in both.
const expansionMark = guardrule.ExpansionMark

// Command is one simple command: its words after quote removal, with
// leading assignments and redirections dropped.
type Command struct {
	Words []Word
	// Line is the command's line in the parsed source, from 1.
	Line int
}

// String writes the command's words separated by spaces, with each
// expansion shown as $….
func (c Command) String() string {
	parts := make([]string, len(c.Words))
	for i, w := range c.Words {
		parts[i] = guardrule.ShowExpansions(w.Text)
	}
	return strings.Join(parts, " ")
}

// Calls reports whether c calls target: their words are equal, word for
// word, and every word is literal (spec 0051, "called").
func (c Command) Calls(target Command) bool {
	if len(c.Words) != len(target.Words) || len(c.Words) == 0 {
		return false
	}
	for i, w := range c.Words {
		t := target.Words[i]
		if !w.Literal || !t.Literal || w.Text != t.Text {
			return false
		}
	}
	return true
}

// HasPrefix reports whether c's first words are literally words, such as
// HasPrefix("vibe", "audit") for C-04 or HasPrefix("npm", "audit") for
// D-03.
func (c Command) HasPrefix(words ...string) bool {
	if len(words) == 0 || len(c.Words) < len(words) {
		return false
	}
	for i, w := range words {
		if !c.Words[i].Literal || c.Words[i].Text != w {
			return false
		}
	}
	return true
}

// Called reports whether an entrypoint is called in a set of places: each
// of its commands is called by some command in cmds (spec 0051, "For a
// list entrypoint, called means every command in the list is called, in
// any steps or jobs").
func Called(entrypoint, cmds []Command) bool {
	return len(entrypoint) > 0 && len(Uncalled(entrypoint, cmds)) == 0
}

// Uncalled returns the entrypoint's commands that no command in cmds
// calls.
func Uncalled(entrypoint, cmds []Command) []Command {
	var out []Command
	for _, want := range entrypoint {
		if !CallsAny(cmds, want) {
			out = append(out, want)
		}
	}
	return out
}

// CallsAny reports whether some command in cmds calls target.
func CallsAny(cmds []Command, target Command) bool {
	for _, c := range cmds {
		if c.Calls(target) {
			return true
		}
	}
	return false
}

// ParseCommands parses src as POSIX shell and returns every simple
// command in it, wherever it sits in the parse tree: in lists, pipelines,
// conditionals, loops, subshells, functions and command substitutions.
// A string passed to sh -c or bash -c stays a word and is not parsed
// again. An assignment with no command is not a command.
func ParseCommands(src string) ([]Command, error) {
	f, err := parseShell(src)
	if err != nil {
		return nil, err
	}
	var out []Command
	syntax.Walk(f, func(n syntax.Node) bool {
		if call, ok := n.(*syntax.CallExpr); ok && len(call.Args) > 0 {
			out = append(out, commandOf(call))
		}
		return true
	})
	return out, nil
}

// ParseSimpleCommand parses src as exactly one simple command: words with
// optional leading assignments, and no operator, pipe, redirection or
// compound command. That is the shape an entrypoint command must have,
// since a command of any other shape can never be called (spec 0052,
// "Entrypoints"). The error says what src is instead.
func ParseSimpleCommand(src string) (Command, error) {
	f, err := parseShell(src)
	if err != nil {
		return Command{}, fmt.Errorf("does not parse as POSIX shell: %w", err)
	}
	if len(f.Stmts) == 0 {
		return Command{}, errors.New("is empty")
	}
	if len(f.Stmts) > 1 {
		return Command{}, errors.New("is a sequence of commands, not one simple command")
	}
	st := f.Stmts[0]
	call, ok := st.Cmd.(*syntax.CallExpr)
	switch {
	case !ok:
		return Command{}, fmt.Errorf("is %s, not one simple command", describeCommand(st.Cmd))
	case st.Negated || st.Background:
		return Command{}, errors.New("uses an operator (! or &), so it is not one simple command")
	case len(st.Redirs) > 0:
		return Command{}, errors.New("has a redirection, so it is not one simple command")
	case len(call.Args) == 0:
		return Command{}, errors.New("only assigns variables and runs no command")
	}
	return commandOf(call), nil
}

// commandAssignments returns the leading assignments of a simple command
// as NAME=value. A value with an expansion, an array or an append needs a
// shell, and why says so.
func commandAssignments(src string) (assigns []string, why string) {
	f, err := parseShell(src)
	if err != nil || len(f.Stmts) != 1 {
		return nil, "it is not one simple command"
	}
	call, ok := f.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok {
		return nil, "it is not one simple command"
	}
	for _, a := range call.Assigns {
		if a.Append || a.Array != nil || a.Index != nil || a.Name == nil {
			return nil, fmt.Sprintf("the assignment to %s is not NAME=value", assignName(a))
		}
		val := Word{Literal: true}
		if a.Value != nil {
			val = wordOf(a.Value)
		}
		if !val.Literal {
			return nil, fmt.Sprintf("the assignment to %s holds a parameter expansion or command substitution", a.Name.Value)
		}
		assigns = append(assigns, a.Name.Value+"="+val.Text)
	}
	return assigns, ""
}

func assignName(a *syntax.Assign) string {
	if a.Name == nil {
		return "a variable"
	}
	return a.Name.Value
}

func describeCommand(cmd syntax.Command) string {
	switch c := cmd.(type) {
	case *syntax.BinaryCmd:
		if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
			return "a pipeline"
		}
		return "a list joined by " + c.Op.String()
	case *syntax.Subshell:
		return "a subshell"
	case *syntax.Block:
		return "a block"
	case *syntax.IfClause:
		return "an if statement"
	case *syntax.WhileClause:
		return "a loop"
	case *syntax.ForClause:
		return "a for loop"
	case *syntax.CaseClause:
		return "a case statement"
	case *syntax.FuncDecl:
		return "a function definition"
	default:
		return "a compound command"
	}
}

func parseShell(src string) (*syntax.File, error) {
	return syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(src), "")
}

func commandOf(call *syntax.CallExpr) Command {
	c := Command{Line: int(call.Args[0].Pos().Line())}
	for _, w := range call.Args {
		c.Words = append(c.Words, wordOf(w))
	}
	return c
}

// wordOf performs quote removal on a word. Any part other than literal
// text and quotes is an expansion.
func wordOf(w *syntax.Word) Word {
	var b strings.Builder
	literal := true
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(guardrule.Unescape(p.Value, false))
		case *syntax.SglQuoted:
			if p.Dollar {
				literal = false
				b.WriteString(expansionMark)
				continue
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				if l, ok := q.(*syntax.Lit); ok {
					b.WriteString(guardrule.Unescape(l.Value, true))
					continue
				}
				literal = false
				b.WriteString(expansionMark)
			}
		default:
			literal = false
			b.WriteString(expansionMark)
		}
	}
	return Word{Text: b.String(), Literal: literal}
}

// githubExpression matches a GitHub Actions expression, ${{ … }}.
var githubExpression = regexp.MustCompile(`(?s)\$\{\{.*?\}\}`)

// githubPlaceholder replaces a GitHub expression before a run: step is
// parsed. It is a parameter expansion, so the word holding it matches no
// entrypoint word, and it does not end in @latest.
const githubPlaceholder = "${VIBE_GITHUB_EXPRESSION}"

// Script is a GitHub Actions run: step as the static checks see it.
type Script struct {
	// Source is the step's run: text as written.
	Source string
	// Commands is every simple command of the step. It is empty when the
	// step does not parse: such a step calls nothing.
	Commands []Command
	// Err is the parse error, or nil.
	Err error
	// Fields is the step split on whitespace, after GitHub expressions
	// are replaced. C-02 checks these words for @latest when the step
	// does not parse (spec 0052, "Shell parsing").
	Fields []string
}

// ParseRunStep parses a run: step. Each ${{ … }} is replaced with a
// placeholder first. A step that does not parse is kept with its error:
// it calls nothing, and C-02 reads its whitespace-separated words.
func ParseRunStep(src string) Script {
	s := githubExpression.ReplaceAllLiteralString(src, githubPlaceholder)
	cmds, err := ParseCommands(s)
	if err != nil {
		return Script{Source: src, Err: err, Fields: strings.Fields(s)}
	}
	return Script{Source: src, Commands: cmds}
}

// ParseShellScript parses one GitLab script line (a before_script, script
// or after_script item) as POSIX shell, kept like a run: step that does
// not parse when it does not. GitLab's $VAR and ${VAR} are already shell
// expansions, so nothing is replaced first.
func ParseShellScript(src string) Script {
	cmds, err := ParseCommands(src)
	if err != nil {
		return Script{Source: src, Err: err, Fields: strings.Fields(src)}
	}
	return Script{Source: src, Commands: cmds}
}

// Words returns every word of the step's simple commands, or the step's
// whitespace-separated fields when it does not parse.
func (s Script) Words() []string {
	if s.Err != nil {
		return s.Fields
	}
	var out []string
	for _, c := range s.Commands {
		for _, w := range c.Words {
			out = append(out, w.Text)
		}
	}
	return out
}
