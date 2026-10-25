package cli

import (
	"bytes"
	"strings"
	"testing"
)

// migrationGuide is where a user of the template-sync system finds the
// migration guide once vibe init --skill has written the setup skill.
const migrationGuide = ".claude/skills/vibeconform-setup/reference/migrate.md"

// The template-sync commands are gone (spec 0059, Desired Behavior 8):
// each prints one line that names the migration guide, and exits 1.
func TestRemovedCommandsPointAtTheMigrationGuide(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"sync", "diff", "check"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeVibeYAML(t, dir, "standard: prod-go\nversion: v1\n")
			root := NewRootCmd("test")
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)
			root.SetArgs([]string{name, "--repo-root", dir})
			err := root.Execute()
			if code := ExitCode(err); code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			all := out.String() + errOut.String()
			if n := strings.Count(strings.TrimRight(all, "\n"), "\n"); n != 0 {
				t.Errorf("printed %d lines, want one:\n%s", n+1, all)
			}
			for _, want := range []string{"vibe " + name, "vibe init --skill", migrationGuide} {
				if !strings.Contains(all, want) {
					t.Errorf("output lacks %q:\n%s", want, all)
				}
			}
			if exists(t, dir, ".vibe") {
				t.Error("the stub wrote .vibe/")
			}
		})
	}
}

// The root help describes the contract model and lists only the commands
// that remain.
func TestRootHelpListsTheContractCommands(t *testing.T) {
	t.Parallel()
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"contract", "init", "audit", "unlock", "doctor"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q:\n%s", want, help)
		}
	}
	for _, gone := range []string{"sync", "diff", "check", "reconcile", "standard"} {
		if strings.Contains(help, gone) {
			t.Errorf("help still names %q:\n%s", gone, help)
		}
	}
}
