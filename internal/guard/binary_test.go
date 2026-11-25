package guard_test

import (
	"strings"
	"testing"
)

// The binary set runs the vibe-guard binary as Claude Code and a person
// do (spec 0070, "The guard's tests"): a payload on its real stdin, the
// exit code from the process. Each call must give exactly what
// guard.Main gives in-process for the same call, so the in-process
// tests speak for the binary. Short mode skips it, as it does every
// subprocess test.

func skipSubprocessInShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode skips this test, which runs vibe-guard as a subprocess; task verify runs it")
	}
}

// TestBinary: one deny, one allow and one ask payload on the binary's
// stdin, and one explain.
func TestBinary(t *testing.T) {
	skipSubprocessInShort(t)
	r := newRepo(t, `{"version": 2, "rules": [{"id": "ask-docker-push", "level": "ask", "command": "docker push", "reason": "pushes an image"}]}`)
	cases := []struct {
		name, stdin string
		args        []string
		check       func(*testing.T, result)
	}{
		{"deny", bashPayload("rm -rf build"), []string{"hook"}, func(t *testing.T, got result) {
			t.Helper()
			assertRefused(t, got, "rm-rf")
		}},
		{"allow", bashPayload("ls"), []string{"hook"}, func(t *testing.T, got result) {
			t.Helper()
			assertAllowed(t, got)
		}},
		{"ask", bashPayload("docker push app:1"), []string{"hook"}, func(t *testing.T, got result) {
			t.Helper()
			assertAsk(t, got, "ask-docker-push")
		}},
		{"explain", "", []string{"explain", "git reset --hard"}, func(t *testing.T, got result) {
			t.Helper()
			if got.Code != 2 || !hasLine(got.Stdout, "decision: deny") || !hasLine(got.Stdout, "rule: git-reset-hard") {
				t.Errorf("exit %d, output\n%s\nwant exit 2, decision deny and rule git-reset-hard", got.Code, got.Stdout)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBinary(t, r.Root, c.stdin, r.projectEnv(), c.args...)
			c.check(t, got)
			if in := runGuard(t, r.Root, c.stdin, r.projectEnv(), c.args...); got != in {
				t.Errorf("the binary gives %+v; guard.Main in-process gives %+v", got, in)
			}
		})
	}
}

// TestVersion: the binary prints its version.
func TestVersion(t *testing.T) {
	skipSubprocessInShort(t)
	got := runBinary(t, t.TempDir(), "", nil, "version")
	if got.Code != 0 || !strings.HasPrefix(got.Stdout, "vibe-guard ") {
		t.Errorf("exit %d, stdout %q", got.Code, got.Stdout)
	}
}
