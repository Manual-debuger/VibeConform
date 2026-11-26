package guard_test

import (
	"maps"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// recordedUsage is vibe-guard's usage text at 2757b85, before spec 0073
// generated it from the command table.
const recordedUsage = `usage:
  vibe-guard hook                                  decide the PreToolUse payload on stdin
  vibe-guard explain [--tool <tool>] '<command or path>'
  vibe-guard relock                                delete the unlock marker
  vibe-guard version
`

// TestUsageIsUnchanged: the usage that help prints, now generated from
// the command table, is the text it was before.
func TestUsageIsUnchanged(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		if got := runGuard(t, t.TempDir(), "", nil, args...); got.Code != 0 || got.Stdout != recordedUsage {
			t.Errorf("%q: exit %d, stdout\n%s\nwant\n%s", args, got.Code, got.Stdout, recordedUsage)
		}
	}
	if got := runGuard(t, t.TempDir(), "", nil); got.Code != 64 || got.Stderr != recordedUsage {
		t.Errorf("no arguments: exit %d, stderr\n%s\nwant\n%s", got.Code, got.Stderr, recordedUsage)
	}
}

// TestCommandTableGivesTheRecordedCommands: the command table gives the
// commands and flags that the skill drift checker read from the help
// text at 2757b85 (command -> flag -> takes a value), with the aliases
// it added by hand (--version, help). The help aliases -h and --help,
// which Main accepted then too but the help text did not show, are now
// in the table as well.
func TestCommandTableGivesTheRecordedCommands(t *testing.T) {
	recorded := map[string]map[string]bool{
		"hook":      {},
		"explain":   {"--tool": true},
		"relock":    {},
		"version":   {},
		"--version": {},
		"help":      {},
		"-h":        {},
		"--help":    {},
	}
	got := map[string]map[string]bool{}
	for _, c := range guard.Commands() {
		flags := map[string]bool{}
		for _, f := range c.Flags {
			flags[f.Name] = f.TakesValue()
		}
		for _, name := range append([]string{c.Name}, c.Aliases...) {
			if _, dup := got[name]; dup {
				t.Errorf("%s names two commands", name)
			}
			got[name] = flags
		}
	}
	if !maps.EqualFunc(got, recorded, maps.Equal) {
		t.Errorf("Commands() = %v, want %v", got, recorded)
	}
}

// TestEveryTableCommandRuns: Main runs every name and alias in the table;
// none is a usage error with no arguments beyond what the command needs.
func TestEveryTableCommandRuns(t *testing.T) {
	needs := map[string][]string{"explain": {"echo hi"}}
	for _, c := range guard.Commands() {
		for _, name := range append([]string{c.Name}, c.Aliases...) {
			args := append([]string{name}, needs[c.Name]...)
			if got := runGuard(t, t.TempDir(), `{"tool_name": "Bash", "tool_input": {"command": "echo hi"}}`, nil, args...); got.Code == 64 {
				t.Errorf("%q: a usage error; stderr %q", args, got.Stderr)
			}
		}
	}
}
