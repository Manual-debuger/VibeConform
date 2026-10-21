package module

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// prePushGitVars are the variables pre-push clears (spec 0057 Desired
// Behavior 9).
var prePushGitVars = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"}

// TestPrePushClearsGitVarsInLinkedWorktree pins spec 0057 acceptance
// criterion 3 against a real git and lefthook: a push from a linked
// worktree runs the generated pre-push, and Git, which exports GIT_DIR to
// the hook there, no longer reaches verify:fast's tests with it. The
// fixture's verify:fast prints the variables instead of running checks.
// The monorepo template's pre-push runs the same command, which
// internal/standard's TestLefthookHookStages pins.
func TestPrePushClearsGitVarsInLinkedWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which pushes through a real lefthook; task verify runs it")
	}
	lefthook, err := exec.LookPath("lefthook")
	if err != nil {
		t.Skip("lefthook not on PATH")
	}
	taskOnPath(t)
	var echo strings.Builder
	for _, v := range prePushGitVars {
		fmt.Fprintf(&echo, "echo \"%s=${%s:-unset}\"\n", v, v)
	}
	taskfile := "version: '3'\ntasks:\n  verify:fast:\n    silent: true\n    cmds:\n      - |\n"
	for line := range strings.Lines(echo.String()) {
		taskfile += "        " + line
	}
	for _, tmpl := range lefthookTemplates {
		t.Run(tmpl, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(tmpl)
			if err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			main := filepath.Join(parent, "main")
			if err := os.Mkdir(main, 0o700); err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{"lefthook.yml": string(data), "Taskfile.yml": taskfile} {
				if err := os.WriteFile(filepath.Join(main, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			gitRepo(t, main)
			remote := filepath.Join(parent, "remote.git")
			side := filepath.Join(parent, "side")
			for _, step := range []struct {
				dir  string
				argv []string
			}{
				{parent, []string{"git", "init", "-q", "--bare", remote}},
				{main, []string{"git", "remote", "add", "origin", remote}},
				{main, []string{lefthook, "install"}},
				{main, []string{"git", "worktree", "add", "-q", "-b", "side", side}},
			} {
				if code, stdout, stderr := runCommand(t, step.dir, "", step.argv...); code != 0 {
					t.Fatalf("%q: exit %d\n%s%s", step.argv, code, stdout, stderr)
				}
			}

			code, stdout, stderr := runCommand(t, side, "", "git", "push", "origin", "side")
			out := stdout + stderr
			if code != 0 {
				t.Fatalf("git push from the linked worktree: exit %d\n%s", code, out)
			}
			for _, v := range prePushGitVars {
				if !strings.Contains(out, v+"=unset") {
					t.Errorf("verify:fast did not see %s unset; output:\n%s", v, out)
				}
			}
		})
	}
}
