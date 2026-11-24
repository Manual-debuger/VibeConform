package guard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A project below the Git top (spec 0068, #180): Claude Code runs in
// top/proj, which has no .git of its own, and sets CLAUDE_PROJECT_DIR to
// it. The guard protects the project's guardrail settings and the Git
// top's, reads the project's policy.json only, and finds the unlock
// marker in the Git top's common Git directory.

// nestedProject makes a fixture repository top with a project top/proj and
// a sibling project top/other. topPolicy and projPolicy are the two
// policy.json texts, or "" for none. proj's CommonDir is top's.
func nestedProject(t *testing.T, topPolicy, projPolicy string) (top, proj repo) {
	t.Helper()
	top = newRepo(t, topPolicy)
	proj = repo{Root: filepath.Join(top.Root, "proj"), CommonDir: top.CommonDir}
	if err := os.MkdirAll(proj.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if projPolicy != "" {
		writeFile(t, filepath.Join(proj.Root, ".claude", "hooks", "policy.json"), projPolicy)
	}
	return top, proj
}

// TestProjectBelowGitTopGuardrailSettings: the project's and the Git
// top's .claude/settings.json and .claude/hooks/ are refused while
// locked; a sibling project's are ordinary files.
func TestProjectBelowGitTopGuardrailSettings(t *testing.T) {
	top, proj := nestedProject(t, "", enforcePolicy)
	refused := []string{
		filepath.Join(proj.Root, ".claude", "settings.json"),
		filepath.Join(proj.Root, ".claude", "hooks", "pre.sh"),
		".claude/settings.json",
		"./.claude/hooks/pre.sh",
		`.claude\hooks\pre.sh`,
		".Claude/Settings.json",
		filepath.Join(top.Root, ".claude", "settings.json"),
		filepath.Join(top.Root, ".claude", "hooks", "pre.sh"),
		"../.claude/settings.json",
		"../.claude/hooks/pre.sh",
	}
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		for _, p := range refused {
			t.Run(tool+" "+p, func(t *testing.T) {
				assertRefused(t, proj.hook(t, editPayload(tool, p, proj.Root)), "guardrail-settings")
			})
		}
	}
	for _, p := range []string{
		filepath.Join(top.Root, "other", ".claude", "settings.json"),
		"../other/.claude/settings.json",
		"../other/.claude/hooks/pre.sh",
		"sub/.claude/settings.json",
	} {
		t.Run("allowed "+p, func(t *testing.T) {
			assertAllowed(t, proj.hook(t, editPayload("Write", p, proj.Root)))
		})
	}
}

// TestProjectBelowGitTopShellWrite: a shell write of the project's
// settings is checked like an edit.
func TestProjectBelowGitTopShellWrite(t *testing.T) {
	_, proj := nestedProject(t, "", "")
	assertRefused(t, proj.hook(t, commandPayload("Bash", "echo {} | tee .claude/settings.json", proj.Root)), "guardrail-settings")
	assertRefused(t, proj.hook(t, commandPayload("Bash", "echo x > .claude/hooks/pre.sh", proj.Root)), "guardrail-settings")
	assertRefused(t, proj.hook(t, commandPayload("Bash", "echo {} > ../.claude/settings.json", proj.Root)), "guardrail-settings")
}

// TestProjectBelowGitTopUnlock: the marker in the Git top's common Git
// directory unlocks the project's and the top's settings; an expired one
// does not; the agent cannot write it.
func TestProjectBelowGitTopUnlock(t *testing.T) {
	top, proj := nestedProject(t, "", "")
	settings := []string{".claude/settings.json", filepath.Join(proj.Root, ".claude", "settings.json"), filepath.Join(top.Root, ".claude", "hooks", "pre.sh")}
	markerFile := filepath.Join(top.CommonDir, "vibe-unlock")

	writeFile(t, markerFile, marker(time.Now().Add(-time.Minute)))
	for _, p := range settings {
		assertRefused(t, proj.hook(t, editPayload("Write", p, proj.Root)), "guardrail-settings")
	}
	writeFile(t, markerFile, marker(time.Now().Add(10*time.Minute)))
	for _, p := range settings {
		assertAllowed(t, proj.hook(t, editPayload("Write", p, proj.Root)))
	}
	assertRefused(t, proj.hook(t, editPayload("Write", markerFile, proj.Root)), "guardrail-settings")
	assertRefused(t, proj.hook(t, editPayload("Write", "../.git/vibe-unlock", proj.Root)), "guardrail-settings")
}

// TestProjectBelowGitTopPolicy: the project's policy.json is the one
// read. The top's rules and exceptions do not apply; an invalid project
// policy refuses every call.
func TestProjectBelowGitTopPolicy(t *testing.T) {
	const topPolicy = `{"version": 2,
		"rules": [{"id": "no-top", "level": "deny", "command": "echo top", "reason": "the top's rule"}],
		"exceptions": [{"rule": "rm-rf", "match": "rm -rf build", "reason": "the top's exception"}]}`
	const projPolicy = `{"version": 2,
		"rules": [{"id": "no-x", "level": "deny", "command": "echo hi", "reason": "the project's rule"}],
		"exceptions": [{"rule": "rm-rf", "match": "rm -rf dist", "reason": "the project's exception"}]}`
	_, proj := nestedProject(t, topPolicy, projPolicy)
	assertRefused(t, proj.hook(t, commandPayload("Bash", "echo hi", proj.Root)), "no-x")
	assertAllowed(t, proj.hook(t, commandPayload("Bash", "echo top", proj.Root)))
	assertRefused(t, proj.hook(t, commandPayload("Bash", "rm -rf build", proj.Root)), "rm-rf")
	assertAllowed(t, proj.hook(t, commandPayload("Bash", "rm -rf dist", proj.Root)))

	_, broken := nestedProject(t, enforcePolicy, `{"version": 2, "rules": [{"id": "x"}]}`)
	assertRefused(t, broken.hook(t, commandPayload("Bash", "ls", broken.Root)), "policy-error")

	_, observe := nestedProject(t, "", `{"version": 2, "mode": "observe"}`)
	if got := observe.hook(t, commandPayload("Bash", "rm -rf build", observe.Root)); got.Code != 0 ||
		!strings.HasPrefix(got.Stderr, "vibe-guard (observe): would refuse by rule rm-rf") {
		t.Errorf("observe mode in the project: exit %d, stderr %q", got.Code, got.Stderr)
	}
}

// TestProjectBelowGitTopExplain: explain decides as the hook does.
func TestProjectBelowGitTopExplain(t *testing.T) {
	_, proj := nestedProject(t, "", "")
	for _, args := range [][]string{
		{"--tool", "Write", filepath.Join(proj.Root, ".claude", "settings.json")},
		{"--tool", "Write", ".claude/settings.json"},
		{"echo {} | tee .claude/settings.json"},
		{"echo {} > ../.claude/settings.json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got := proj.explain(t, args...)
			if got.Code != 2 || !hasLine(got.Stdout, "rule: guardrail-settings") {
				t.Errorf("exit %d, output\n%s", got.Code, got.Stdout)
			}
		})
	}
}

// TestProjectBelowGitTopObserveLog: in observe mode, the log goes to the
// Git top's common Git directory (story 15).
func TestProjectBelowGitTopObserveLog(t *testing.T) {
	_, proj := nestedProject(t, "", `{"version": 2, "mode": "observe"}`)
	got := proj.hook(t, commandPayload("Bash", "git reset --hard", proj.Root))
	if got.Code != 0 || !strings.Contains(got.Stderr, "rule git-reset-hard") {
		t.Fatalf("exit %d, stderr %q", got.Code, got.Stderr)
	}
	log, err := os.ReadFile(filepath.Join(proj.CommonDir, "vibe-guard.log"))
	if err != nil || string(log) != got.Stderr {
		t.Errorf("log %q (%v); want %q", log, err, got.Stderr)
	}
}

// TestProjectBelowGitTopRelock: relock with CLAUDE_PROJECT_DIR set to
// the project deletes the marker in the Git top's .git.
func TestProjectBelowGitTopRelock(t *testing.T) {
	top, proj := nestedProject(t, "", "")
	markerFile := filepath.Join(top.CommonDir, "vibe-unlock")
	writeFile(t, markerFile, marker(time.Now().Add(10*time.Minute)))
	got := runGuard(t, proj.Root, "", []string{"CLAUDE_PROJECT_DIR=" + proj.Root}, "relock")
	if got.Code != 0 || got.Stderr != "" {
		t.Fatalf("exit %d, stderr %q", got.Code, got.Stderr)
	}
	if _, err := os.Stat(markerFile); !os.IsNotExist(err) {
		t.Errorf("the marker is still there: %v", err)
	}
}

// TestProjectBelowGitTopAgreesWithGit: the marker written where
// git rev-parse --git-common-dir, run in the project, points (as
// vibe unlock writes it) unlocks the project's settings.
func TestProjectBelowGitTopAgreesWithGit(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not on PATH")
	}
	top := t.TempDir()
	proj := filepath.Join(top, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(gitBin, "-C", top, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	out, err := exec.Command(gitBin, "-C", proj, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	common := filepath.FromSlash(strings.TrimSpace(string(out)))
	if !filepath.IsAbs(common) {
		common = filepath.Join(proj, common)
	}
	r := repo{Root: proj}
	assertRefused(t, r.hook(t, editPayload("Write", filepath.Join(proj, ".claude", "settings.json"), proj)), "guardrail-settings")
	writeFile(t, filepath.Join(common, "vibe-unlock"), marker(time.Now().Add(10*time.Minute)))
	assertAllowed(t, r.hook(t, editPayload("Write", filepath.Join(proj, ".claude", "settings.json"), proj)))
}
