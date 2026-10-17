package guard

import (
	"path"
	"regexp"
	"slices"
	"strings"

	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The baseline rules (spec 0053, "Baseline rules"): one per guard rule
// ID of contract 1.0.0. They are built in; policy.json can lift one with
// an exception, never remove it.
//
// Every command rule reads a simple command's words. A word that holds
// an unquoted expansion (Word.Unknown) can become any number of words,
// flags included, so it counts as matching (spec 0053, "Unknown
// words"). A word that holds only quoted expansions is one operand whose
// value is not known: it is never a flag, but where a rule reads an
// operand's value (a +refspec, a whole-tree pathspec) it counts as
// matching too, because the guard fails closed.

type commandRule struct {
	id  string
	why string
	// match reports whether the rule refuses the simple command.
	match func(c gr.Command) bool
}

type pathRule struct {
	id    string
	why   string
	match func(p string) bool
}

const askUser = "; ask the user to run it themselves (in Claude Code: ! <command>)"

var (
	rmRF = commandRule{gr.RmRf, "rm with -r and -f deletes a directory tree without asking" + askUser, matchRmRF}

	removeItemRF = commandRule{gr.RemoveItemRecurseForce,
		"Remove-Item with -Recurse and -Force deletes a directory tree without asking" + askUser, matchRemoveItemRF}

	gitResetHard = commandRule{gr.GitResetHard, "git reset --hard discards uncommitted work" + askUser,
		gitRule("reset", matchResetHard)}

	gitCleanFD = commandRule{gr.GitCleanFD, "git clean with -f and -d deletes untracked files and directories" + askUser,
		gitRule("clean", matchCleanFD)}

	gitPushForce = commandRule{gr.GitPushForce,
		"git push --force (or -f, or a +refspec) can overwrite remote history; --force-with-lease is allowed" + askUser,
		gitRule("push", matchPushForce)}

	gitBranchForceDelete = commandRule{gr.GitBranchForceDelete,
		"git branch -D (or --delete --force) deletes a branch that may not be merged" + askUser,
		gitRule("branch", matchBranchForceDelete)}

	gitDiscardWorktree = commandRule{gr.GitDiscardWorktree,
		"git restore . (or git checkout -- .) discards uncommitted changes in the whole working tree" + askUser,
		matchDiscardWorktree}

	secretFile = pathRule{gr.SecretFile,
		"this looks like a secret or credential file; ask the user to edit it themselves", matchSecretFile}

	guardPolicyFile = pathRule{gr.GuardPolicyFile,
		"the guard's policy files are the user's to change, an exception above all; ask the user to edit it themselves",
		matchGuardPolicyFile}
)

// commandRules lists the command rules in the order a command is checked
// against them. In PowerShell, rm is the Remove-Item alias on Windows and
// the rm program elsewhere, so a PowerShell rm is read both ways, as
// Remove-Item first.
var commandRules = []commandRule{
	rmRF, removeItemRF, gitResetHard, gitCleanFD, gitPushForce, gitBranchForceDelete, gitDiscardWorktree,
}

func commandRulesFor(shell gr.Shell) []commandRule {
	if shell != gr.ShellPowerShell {
		return commandRules
	}
	out := []commandRule{removeItemRF, rmRF}
	for _, r := range commandRules {
		if r.id != gr.RmRf && r.id != gr.RemoveItemRecurseForce {
			out = append(out, r)
		}
	}
	return out
}

var pathRules = []pathRule{secretFile, guardPolicyFile}

// posixArgs is a POSIX-style argument list read by getopt's conventions:
// short options cluster (-rf), long options may be abbreviated and carry
// =value, options may follow operands, and "--" ends them.
type posixArgs struct {
	shorts   string
	longs    []string
	operands []gr.Word
	// unknown is set when a word holds an unquoted expansion before "--":
	// it can be any option.
	unknown bool
}

// scanPosix reads words. withArg lists the short options that take an
// argument; longWithArg the long ones (without "--") that take one as a
// separate word.
func scanPosix(words []gr.Word, withArg string, longWithArg ...string) posixArgs {
	var a posixArgs
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case w.Unknown:
			a.unknown = true
			a.operands = append(a.operands, w)
		case !w.Literal:
			a.operands = append(a.operands, w) // a quoted expansion: one operand
		case w.Text == "--":
			a.operands = append(a.operands, words[i+1:]...)
			return a
		case strings.HasPrefix(w.Text, "--"):
			name, _, hasValue := strings.Cut(w.Text[2:], "=")
			a.longs = append(a.longs, name)
			if !hasValue && slices.Contains(longWithArg, name) {
				i++
			}
		case len(w.Text) > 1 && w.Text[0] == '-':
			for j := 1; j < len(w.Text); j++ {
				a.shorts += w.Text[j : j+1]
				if strings.IndexByte(withArg, w.Text[j]) >= 0 {
					if j == len(w.Text)-1 {
						i++
					}
					break
				}
			}
		default:
			a.operands = append(a.operands, w)
		}
	}
	return a
}

// short reports whether any of the short options in set was given.
func (a posixArgs) short(set string) bool { return strings.ContainsAny(a.shorts, set) }

// long reports whether a long option was given that abbreviates full
// with at least minLen characters (the shortest prefix the program
// accepts as that option).
func (a posixArgs) long(full string, minLen int) bool {
	return slices.ContainsFunc(a.longs, func(l string) bool {
		return len(l) >= minLen && strings.HasPrefix(full, l)
	})
}

func matchRmRF(c gr.Command) bool {
	if c.Name() != "rm" {
		return false
	}
	a := scanPosix(c.Words[1:], "")
	recursive := a.short("rR") || a.long("recursive", 1)
	force := a.short("f") || a.long("force", 1)
	return a.unknown || recursive && force
}

func matchRemoveItemRF(c gr.Command) bool {
	if c.Shell != gr.ShellPowerShell || c.Cmdlet() != "remove-item" {
		return false
	}
	var recurse, force, unknown bool
	for _, w := range c.Words[1:] {
		if w.Unknown {
			unknown = true
			continue
		}
		// -f alone is ambiguous with -Filter, so -Force needs -fo.
		recurse = recurse || psSwitch(w, "Recurse", 1)
		force = force || psSwitch(w, "Force", 2)
	}
	return (recurse || unknown) && (force || unknown)
}

// psSwitch reports whether w turns on the switch parameter param, given
// with at least minLen letters: -Recurse, -rec, -Recurse:$true, but not
// -Recurse:$false.
func psSwitch(w gr.Word, param string, minLen int) bool {
	ok, value := gr.PSParam(w, param)
	if !ok {
		return false
	}
	name, _, _ := strings.Cut(w.Text[1:], ":")
	if len(name) < minLen {
		return false
	}
	v := strings.ToLower(value)
	return v != "$false" && v != "0" && v != "$null"
}

// gitGlobalWithArg are git's global options that take a separate
// argument.
var gitGlobalWithArg = []string{"--git-dir", "--work-tree", "--namespace", "--super-prefix", "--config-env", "-C", "-c"}

// gitSub finds git's subcommand and its arguments. known is false when a
// word with an expansion stands where the subcommand may be: it can be
// any subcommand.
func gitSub(c gr.Command) (sub string, args []gr.Word, known bool) {
	if c.Name() != "git" {
		return "", nil, true
	}
	words := c.Words[1:]
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case !w.Literal:
			return "", words[i+1:], false
		case slices.Contains(gitGlobalWithArg, w.Text):
			i++
		case strings.HasPrefix(w.Text, "-"):
		default:
			return w.Text, words[i+1:], true
		}
	}
	return "", nil, true
}

// gitRule builds a rule on one git subcommand. A git command whose
// subcommand is not known counts as matching.
func gitRule(sub string, match func(args []gr.Word) bool) func(gr.Command) bool {
	return func(c gr.Command) bool {
		if c.Name() != "git" {
			return false
		}
		s, args, known := gitSub(c)
		if !known {
			return true
		}
		return s == sub && match(args)
	}
}

func matchResetHard(args []gr.Word) bool {
	a := scanPosix(args, "")
	return a.unknown || a.long("hard", 2)
}

func matchCleanFD(args []gr.Word) bool {
	a := scanPosix(args, "e", "exclude")
	return a.unknown || (a.short("f") || a.long("force", 1)) && a.short("d")
}

func matchPushForce(args []gr.Word) bool {
	a := scanPosix(args, "o", "push-option", "repo", "receive-pack", "exec")
	if a.unknown || a.short("f") || slices.Contains(a.longs, "force") {
		return true
	}
	for _, w := range a.operands {
		if strings.HasPrefix(w.Text, "+") || strings.HasPrefix(w.Text, gr.ExpansionMark) {
			return true
		}
	}
	return false
}

func matchBranchForceDelete(args []gr.Word) bool {
	a := scanPosix(args, "u", "set-upstream-to", "contains", "no-contains", "points-at", "sort", "format", "color", "column")
	if a.unknown || strings.Contains(a.shorts, "D") {
		return true
	}
	// --for is ambiguous with --format, so --force needs --forc.
	return (a.short("d") || a.long("delete", 1)) && (a.short("f") || a.long("force", 4))
}

func matchDiscardWorktree(c gr.Command) bool {
	if c.Name() != "git" {
		return false
	}
	sub, args, known := gitSub(c)
	switch {
	case !known:
		return true
	case sub == "restore":
		a := scanPosix(args, "s", "source")
		staged := a.short("S") || a.long("staged", 2)
		worktree := a.short("W") || a.long("worktree", 1)
		if staged && !worktree && !a.unknown {
			return false // only unstages
		}
		return a.unknown || slices.ContainsFunc(a.operands, wholeTree)
	case sub == "checkout":
		a := scanPosix(args, "bB", "orphan")
		return a.unknown || slices.ContainsFunc(a.operands, wholeTree)
	}
	return false
}

// pathspecMagic is a leading :/ or :(…) of a Git pathspec.
var pathspecMagic = regexp.MustCompile(`^:(/|\([^)]*\))`)

// wholeTree reports whether a pathspec names the whole working tree: .,
// ./, :/, :(top), * and their spellings. A pathspec with an expansion
// may.
func wholeTree(w gr.Word) bool {
	if !w.Literal {
		return true
	}
	p := w.Text
	top := false
	if m := pathspecMagic.FindString(p); m != "" {
		p, top = p[len(m):], true
	}
	if p == "" {
		return top
	}
	p = path.Clean(p)
	return p == "." || p == "*" || top && p == "/"
}

var secretFileRe = regexp.MustCompile(`(\.env(\.[a-zA-Z]+)?|\.pem|\.key|id_rsa|id_ed25519|credentials\.json|\.npmrc|\.netrc)$`)

func matchSecretFile(p string) bool {
	return secretFileRe.MatchString(strings.ReplaceAll(p, `\`, "/"))
}

var guardPolicyRe = regexp.MustCompile(`(?i)(^|/)\.claude/+hooks/+policy(\.local)?\.json$`)

func matchGuardPolicyFile(p string) bool {
	return guardPolicyRe.MatchString(strings.ReplaceAll(p, `\`, "/"))
}
