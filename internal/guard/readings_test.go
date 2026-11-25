package guard_test

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	gr "github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The two readings of an exception (spec 0070; spec 0053, "Guard rule IDs
// and exceptions, shared"). vibe-guard lifts a rule for the simple command
// or the path that rule refused; the audit's A-02 reads an exception
// through Exception.Matches. The audit's reading must be at least as wide
// as the guard's: a lift the audit does not accept makes the audit fail a
// repository whose guard honours a valid exception.

// builtException is one exception the two-readings test builds for a
// case, and what it was built from.
type builtException struct {
	gr.Exception
	shellWrite bool // a path exception built from a literal shell write
}

// readingsTally counts what the two-readings test ran.
type readingsTally struct {
	cases, exceptions, shellWrites atomic.Int64
	lifts, commandLifts, pathLifts atomic.Int64
}

// TestTwoReadingsOfAnException runs every deny case of the corpus through
// "vibe-guard hook" once for each exception built for it, with a policy
// that holds that exception alone. Whenever the guard lets the case
// through, the audit's reading (Exception.Matches) must accept that
// exception for the case. The exceptions built for a case are: for a
// command rule, each word prefix of each simple command of the case that
// holds no expansion; for a path rule, the path the case edits and the
// path of each literal shell write of the case, made project-relative;
// for any path rule, "**".
//
// A failure is a lift the audit does not accept. The fix belongs in
// Exception.Matches, a contract PATCH under spec 0051.
func TestTwoReadingsOfAnException(t *testing.T) {
	var tally readingsTally
	t.Run("corpus", func(t *testing.T) {
		for _, c := range contract.GuardCorpus() {
			if c.Want != contract.WantDeny {
				continue
			}
			t.Run(c.Name, func(t *testing.T) {
				t.Parallel()
				checkReadings(t, c, &tally)
			})
		}
	})
	if t.Failed() {
		return
	}
	t.Logf("%d deny cases, %d exceptions (%d from shell writes), %d lifts (%d command, %d path)",
		tally.cases.Load(), tally.exceptions.Load(), tally.shellWrites.Load(), tally.lifts.Load(),
		tally.commandLifts.Load(), tally.pathLifts.Load())
	// The test must not pass vacuously: the guard lifts command and path
	// rules by the exceptions built here, and shell writes are read.
	if tally.commandLifts.Load() == 0 || tally.pathLifts.Load() == 0 {
		t.Errorf("the guard lifted %d command and %d path cases; want some of each",
			tally.commandLifts.Load(), tally.pathLifts.Load())
	}
	if tally.shellWrites.Load() == 0 {
		t.Error("no path exception was built from a shell write")
	}
}

// checkReadings runs c with each exception built for it.
func checkReadings(t *testing.T, c contract.GuardCase, tally *readingsTally) {
	t.Helper()
	r := corpusRepo(t, c, testNow)
	tally.cases.Add(1)
	for _, e := range exceptionsBuiltFor(t, c, r.Root) {
		tally.exceptions.Add(1)
		if e.shellWrite {
			tally.shellWrites.Add(1)
		}
		writeFile(t, filepath.Join(r.Root, ".claude", "hooks", "policy.json"), policyWith(t, e.Exception))
		got := r.hook(t, payloadFor(t, c, r.Root))
		switch {
		case got.Code == 2 && strings.HasPrefix(got.Stderr, "vibe-guard: refused by rule "+gr.IDPolicyError+":"):
			t.Fatalf("exception %+v: the policy does not load: %s", e.Exception, got.Stderr)
		case got.Code == 2:
			continue // the guard does not lift the case by this exception
		case got.Code != 0:
			t.Fatalf("exception %+v: exit %d, stderr %q", e.Exception, got.Code, got.Stderr)
		}
		tally.lifts.Add(1)
		if gr.IsPathRule(e.Rule) {
			tally.pathLifts.Add(1)
		} else {
			tally.commandLifts.Add(1)
		}
		if !e.Matches(c.Target(), r.Root) {
			t.Errorf("vibe-guard lets the case through by exception {rule %q, match %q}, which the audit's reading "+
				"(Exception.Matches) does not accept; widen Exception.Matches (spec 0070, \"The two readings of an exception\")",
				e.Rule, e.Match)
		}
	}
}

// exceptionsBuiltFor builds the exceptions for c's rule, with root as
// the project root.
func exceptionsBuiltFor(t *testing.T, c contract.GuardCase, root string) []builtException {
	t.Helper()
	tgt := c.Target()
	var cmds []gr.Command
	if gr.IsCommandTool(tgt.Tool) {
		var err error
		if cmds, err = gr.Commands(gr.ShellFor(tgt.Tool), tgt.Command); err != nil {
			t.Fatalf("a deny case for rule %s does not parse: %v", c.Rule, err)
		}
	}
	exceptionFor := func(match string) gr.Exception {
		return gr.Exception{Rule: c.Rule, Match: match, Reason: "two readings"}
	}
	if !gr.IsPathRule(c.Rule) {
		return commandExceptions(t, cmds, exceptionFor)
	}
	var out []builtException
	if gr.IsEditTool(tgt.Tool) {
		out = append(out, builtException{Exception: exceptionFor(gr.RelPath(root, tgt.Path))})
	}
	for _, cmd := range cmds {
		for _, w := range cmd.Writes {
			if w.Literal {
				out = append(out, builtException{Exception: exceptionFor(gr.RelPath(root, w.Text)), shellWrite: true})
			}
		}
	}
	return append(out, builtException{Exception: exceptionFor("**")})
}

// commandExceptions builds an exception from each word prefix of each
// simple command of cmds that holds no expansion.
func commandExceptions(t *testing.T, cmds []gr.Command, exceptionFor func(string) gr.Exception) []builtException {
	t.Helper()
	var out []builtException
	for _, cmd := range cmds {
		if !allLiteral(cmd.Words) {
			continue
		}
		for k := 1; k <= len(cmd.Words); k++ {
			out = append(out, builtException{Exception: exceptionFor(commandMatch(t, cmd, k))})
		}
	}
	return out
}

func allLiteral(words []gr.Word) bool {
	for _, w := range words {
		if !w.Literal {
			return false
		}
	}
	return true
}

var (
	bashBare = regexp.MustCompile(`^[A-Za-z0-9_./:+,@%-]+$`)
	psBare   = regexp.MustCompile(`^[A-Za-z0-9_./:\\+,%-]+$`)
)

// commandMatch writes the first k words of cmd as an exception match in
// cmd's language: a word with no special character as it is, any other
// word single-quoted. A PowerShell command name that needs quoting is
// written after the call operator (& 'name'), because a quoted first
// word alone is a string expression, not a command. The match must parse
// back to the same words (its first simple command; a match such as
// "bash -c 'rm -rf x'" also holds the commands it runs).
func commandMatch(t *testing.T, cmd gr.Command, k int) string {
	t.Helper()
	parts := make([]string, k)
	for i, w := range cmd.Words[:k] {
		switch {
		case cmd.Shell == gr.ShellPowerShell && psBare.MatchString(w.Text):
			parts[i] = w.Text
		case cmd.Shell == gr.ShellPowerShell && i == 0:
			parts[i] = "& '" + strings.ReplaceAll(w.Text, "'", "''") + "'"
		case cmd.Shell == gr.ShellPowerShell:
			parts[i] = "'" + strings.ReplaceAll(w.Text, "'", "''") + "'"
		case bashBare.MatchString(w.Text):
			parts[i] = w.Text
		default:
			parts[i] = "'" + strings.ReplaceAll(w.Text, "'", `'\''`) + "'"
		}
	}
	match := strings.Join(parts, " ")
	back, err := gr.Commands(cmd.Shell, match)
	if err != nil || len(back) == 0 || len(back[0].Words) != k || len(back[0].Redirects) > 0 {
		t.Fatalf("the match %q built from %q does not parse back to %d words: %v", match, cmd.String(), k, err)
	}
	for i, w := range back[0].Words {
		if !w.Literal || w.Text != cmd.Words[i].Text {
			t.Fatalf("the match %q built from %q parses back to word %d %q", match, cmd.String(), i, w.Text)
		}
	}
	return match
}

// policyWith is a policy.json that holds e alone.
func policyWith(t *testing.T, e gr.Exception) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"version": 2, "mode": "enforce", "rules": []any{},
		"exceptions": []map[string]string{{"rule": e.Rule, "match": e.Match, "reason": e.Reason}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
