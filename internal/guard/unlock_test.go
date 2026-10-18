package guard_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The guardrail-settings rule beyond the contract corpus (spec 0053,
// "guardrail-settings"): exceptions, marker contents, linked worktrees
// and a Git directory the guard cannot read.

func writePayload(path string) string {
	data, _ := json.Marshal(map[string]any{"tool_name": "Write", "tool_input": map[string]string{"file_path": path, "content": "{}"}})
	return string(data)
}

func marker(at time.Time) string { return at.UTC().Format(time.RFC3339) + "\n" }

func assertRefused(t *testing.T, got result, rule string) {
	t.Helper()
	want := "vibe-guard: refused by rule " + rule + ": "
	if got.Code != 2 || !strings.HasPrefix(got.Stderr, want) {
		t.Errorf("exit %d, stderr %q; want exit 2 and a line starting %q", got.Code, got.Stderr, want)
	}
}

func assertAllowed(t *testing.T, got result) {
	t.Helper()
	if got.Code != 0 || got.Stderr != "" || got.Stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q; want a silent exit 0", got.Code, got.Stdout, got.Stderr)
	}
}

// TestGuardrailSettingsException: an exception for guardrail-settings
// lifts a settings or hooks edit in the guard (the audit's A-08 does not
// count it), but never a self-unlock: the marker and vibe unlock stay
// refused.
func TestGuardrailSettingsException(t *testing.T) {
	r := newRepo(t, `{"version": 2, "exceptions": [
		{"rule": "guardrail-settings", "match": "**", "reason": "the agent maintains its own hooks"},
		{"rule": "guardrail-settings", "match": "vibe", "reason": "never lifts vibe unlock"}]}`)
	assertAllowed(t, r.hook(t, writePayload(filepath.Join(r.Root, ".claude", "settings.json"))))
	assertAllowed(t, r.hook(t, writePayload(".claude/hooks/pre.sh")))
	assertRefused(t, r.hook(t, writePayload(".git/vibe-unlock")), "guardrail-settings")
	assertRefused(t, r.hook(t, bashPayload("vibe unlock")), "guardrail-settings")
	assertRefused(t, r.hook(t, bashPayload("touch .git/vibe-unlock")), "guardrail-settings")
	assertRefused(t, r.hook(t, writePayload(".claude/hooks/policy.json")), "guard-policy-file")
}

// TestGuardrailSettingsOnlyInTheRepository is a regression test: the
// rule guards the repository's own .claude/settings.json and
// .claude/hooks/ (spec 0051), not the same names elsewhere. Before, a
// path holding .claude/settings.json anywhere was refused, so the agent
// could not change the person's own ~/.claude/settings.json.
func TestGuardrailSettingsOnlyInTheRepository(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	home := t.TempDir()
	for _, p := range []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "hooks", "pre.sh"),
		filepath.Join(filepath.Dir(r.Root), ".claude", "settings.json"),
		"../.claude/settings.json",
		"sub/.claude/settings.json",
	} {
		t.Run(p, func(t *testing.T) {
			assertAllowed(t, r.hook(t, writePayload(p)))
		})
	}
	for _, p := range []string{
		filepath.Join(r.Root, ".claude", "settings.json"),
		filepath.Join(r.Root, ".claude", "hooks", "pre.sh"),
		".claude/settings.json",
		"./.claude/hooks/pre.sh",
		`.claude\hooks\pre.sh`,
		".Claude/Settings.json",
	} {
		t.Run(p, func(t *testing.T) {
			assertRefused(t, r.hook(t, writePayload(p)), "guardrail-settings")
		})
	}
}

// TestUnlockMarkerContents: only one RFC 3339 time in the future is a
// valid marker.
func TestUnlockMarkerContents(t *testing.T) {
	cases := []struct {
		name, content string
		valid         bool
	}{
		{"valid", marker(time.Now().Add(10 * time.Minute)), true},
		{"valid without newline", strings.TrimSpace(marker(time.Now().Add(10 * time.Minute))), true},
		{"valid in another zone", time.Now().Add(10 * time.Minute).In(time.FixedZone("x", 3600)).Format(time.RFC3339), true},
		{"expired", marker(time.Now().Add(-time.Minute)), false},
		{"empty", "", false},
		{"not a time", "forever\n", false},
		{"a date only", "2099-01-01\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newRepo(t, enforcePolicy)
			writeFile(t, filepath.Join(r.CommonDir, "vibe-unlock"), c.content)
			got := r.hook(t, writePayload(".claude/settings.json"))
			if c.valid {
				assertAllowed(t, got)
			} else {
				assertRefused(t, got, "guardrail-settings")
			}
		})
	}
}

// linkedWorktree makes a main repository whose Git directory is
// commonName (".git", or "repo.git" for a bare one) and a linked
// worktree of it, as git worktree add lays them out.
func linkedWorktree(t *testing.T, commonName string) (wt repo, common string) {
	t.Helper()
	base := t.TempDir()
	common = filepath.Join(base, commonName)
	gitDir := filepath.Join(common, "worktrees", "wt")
	writeFile(t, filepath.Join(gitDir, "commondir"), "../..\n")
	root := filepath.Join(base, "wt")
	writeFile(t, filepath.Join(root, ".git"), "gitdir: "+filepath.ToSlash(gitDir)+"\n")
	return repo{Root: root, CommonDir: common}, common
}

// TestUnlockInLinkedWorktree: the marker in the common Git directory
// unlocks every worktree, and a write to it is refused even when that
// directory is not named .git.
func TestUnlockInLinkedWorktree(t *testing.T) {
	wt, common := linkedWorktree(t, "repo.git")
	assertRefused(t, wt.hook(t, writePayload(".claude/settings.json")), "guardrail-settings")
	assertRefused(t, wt.hook(t, writePayload(filepath.Join(common, "vibe-unlock"))), "guardrail-settings")
	assertRefused(t, wt.hook(t, writePayload(filepath.Join(common, "worktrees", "wt", "vibe-unlock"))), "guardrail-settings")
	assertAllowed(t, wt.hook(t, writePayload(filepath.Join(wt.Root, "vibe-unlock"))))

	writeFile(t, filepath.Join(common, "vibe-unlock"), marker(time.Now().Add(10*time.Minute)))
	assertAllowed(t, wt.hook(t, writePayload(".claude/settings.json")))
	assertRefused(t, wt.hook(t, writePayload(filepath.Join(common, "vibe-unlock"))), "guardrail-settings")
}

// TestUnlockUnknownGitDir: when the guard cannot read where the common
// Git directory is, no marker is valid.
func TestUnlockUnknownGitDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "not a gitdir line\n")
	r := repo{Root: root}
	assertRefused(t, r.hook(t, writePayload(".claude/settings.json")), "guardrail-settings")
}

// TestVibeWithAnUnknownSubcommand: vibe $x may be vibe unlock.
func TestVibeWithAnUnknownSubcommand(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	assertRefused(t, r.hook(t, bashPayload(`vibe $cmd`)), "guardrail-settings")
	assertRefused(t, r.hook(t, bashPayload(`vibe "$cmd" --minutes 5`)), "guardrail-settings")
	assertAllowed(t, r.hook(t, bashPayload(`vibe sync --dry-run`)))
}
