package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/state"
)

func TestMain(m *testing.M) {
	// The suite plans repositories in t.TempDir(), which is not a git work
	// tree, so the real check would only ever warn. Tests of the check
	// itself call gitCheckIgnored directly or install their own stub.
	checkIgnored = func(context.Context, string, []string) (map[string]bool, error) { return nil, nil }
	os.Exit(m.Run())
}

func writeVibeYAML(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runDiffIn(t *testing.T, dir string) string {
	t.Helper()
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"diff", "--repo-root", dir})
	if err := root.Execute(); err != nil {
		t.Fatalf("diff: %v", err)
	}
	return out.String()
}

func exists(t *testing.T, dir, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

const (
	goDefaults   = "standard: prod-go\nversion: v1\n"
	goClaudeOnly = "standard: prod-go\nversion: v1\nintegrations:\n  agents: [claude]\n"
	goNoAgents   = "standard: prod-go\nversion: v1\nintegrations:\n  agents: []\n"
)

// syncedDefaults is a prod-go repository synced with the default
// integrations, as every repository was before spec 0026.
func syncedDefaults(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDefaults)
	if out, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("seeding sync: %v\n%s", err, out)
	}
	return dir
}

func TestDeselectRemovesUnchangedFiles(t *testing.T) {
	dir := syncedDefaults(t)
	writeVibeYAML(t, dir, goClaudeOnly)

	diff := runDiffIn(t, dir)
	for _, want := range []string{
		".codex/config.toml: would remove (codex deselected)",
		".codex/hooks.json: would remove (codex deselected)",
	} {
		if !strings.Contains(diff, want) {
			t.Errorf("diff missing %q\n%s", want, diff)
		}
	}
	if out, err := runAuditIn(t, dir); ExitCode(err) != exitOutOfDate {
		t.Errorf("audit exit %d, want %d (out of date)\n%s", ExitCode(err), exitOutOfDate, out)
	}

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, want := range []string{".codex/config.toml: removed (codex deselected)", ", 2 removed"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync output missing %q\n%s", want, out)
		}
	}
	if exists(t, dir, ".codex") {
		t.Error(".codex/ left behind, empty")
	}
	if !exists(t, dir, ".claude/settings.json") {
		t.Error("claude's files went with codex's")
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for key := range s.Resources {
		if strings.HasPrefix(key, ".codex/") {
			t.Errorf("state still records %s", key)
		}
	}

	// Idempotent: a second run has nothing left to remove.
	out, err = runSyncIn(t, dir)
	if err != nil || strings.Contains(out, ".codex") || strings.Contains(out, "removed") {
		t.Errorf("second sync not a no-op for removals (err %v)\n%s", err, out)
	}
	if out, err := runAuditIn(t, dir); err != nil {
		t.Errorf("audit after removal: %v\n%s", err, out)
	}

	// Re-selecting recreates through the ordinary Create path.
	writeVibeYAML(t, dir, goDefaults)
	if out, err := runSyncIn(t, dir); err != nil || !strings.Contains(out, ".codex/config.toml: created") {
		t.Errorf("re-select did not recreate (err %v)\n%s", err, out)
	}
}

// TestDeselectKeepsModifiedFile: a file edited since sync is someone's
// work, so deselection keeps it and says so, and sync exits non-zero.
func TestDeselectKeepsModifiedFile(t *testing.T) {
	dir := syncedDefaults(t)
	edited := filepath.Join(dir, ".codex", "config.toml")
	if err := os.WriteFile(edited, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeVibeYAML(t, dir, goClaudeOnly)

	if out, err := runAuditIn(t, dir); ExitCode(err) != exitNonConformant ||
		!strings.Contains(out, ".codex/config.toml: conflict: codex deselected but file modified since sync") {
		t.Errorf("audit exit %d\n%s", ExitCode(err), out)
	}
	out, err := runSyncIn(t, dir)
	if err == nil {
		t.Fatalf("sync succeeded over a modified deselected file\n%s", out)
	}
	if data, _ := os.ReadFile(edited); string(data) != "# mine\n" {
		t.Errorf("modified file changed or removed: %q", data)
	}
	if exists(t, dir, ".codex/hooks.json") {
		t.Error("the unmodified sibling was not removed")
	}
	s, _ := state.Load(dir)
	if _, ok := s.Resources[".codex/config.toml"]; !ok {
		t.Error("the kept file lost its state entry, so it would never be reported again")
	}
}

func TestDeselectForgetsMissingFile(t *testing.T) {
	dir := syncedDefaults(t)
	if err := os.Remove(filepath.Join(dir, ".codex", "hooks.json")); err != nil {
		t.Fatal(err)
	}
	writeVibeYAML(t, dir, goClaudeOnly)

	if diff := runDiffIn(t, dir); !strings.Contains(diff, ".codex/hooks.json: would forget (codex deselected; already removed)") {
		t.Errorf("diff\n%s", diff)
	}
	if out, err := runSyncIn(t, dir); err != nil || !strings.Contains(out, ".codex/hooks.json: forgotten") {
		t.Fatalf("sync (err %v)\n%s", err, out)
	}
	s, _ := state.Load(dir)
	if _, ok := s.Resources[".codex/hooks.json"]; ok {
		t.Error("forgotten entry still in state")
	}
}

// TestDeselectNeverDeletesUnrecordedFiles: without a state entry, the file
// is not VibeConform's to delete, whatever its path.
func TestDeselectNeverDeletesUnrecordedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(dir, ".codex", "config.toml")
	if err := os.WriteFile(own, []byte("# hand-written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeVibeYAML(t, dir, goClaudeOnly)
	if out, err := runSyncIn(t, dir); err != nil || strings.Contains(out, ".codex") {
		t.Fatalf("sync touched an unrecorded file (err %v)\n%s", err, out)
	}
	if data, _ := os.ReadFile(own); string(data) != "# hand-written\n" {
		t.Errorf("unrecorded file changed: %q", data)
	}
}

// TestDeselectAllAgents: agents: [] removes both agents' files, the guard
// the core module emitted for claude, and updates Taskfile.yml, while a
// user's file in .claude/ keeps that directory.
func TestDeselectAllAgents(t *testing.T) {
	dir := syncedDefaults(t)
	local := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.WriteFile(local, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeVibeYAML(t, dir, goNoAgents)

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, want := range []string{
		"Taskfile.yml: updated",
		".claude/hooks/guard.go: removed (claude deselected)",
		".claude/settings.json: removed (claude deselected)",
		".claude/hooks/policy.json: removed (claude deselected)",
		".codex/config.toml: removed (codex deselected)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync output missing %q\n%s", want, out)
		}
	}
	if exists(t, dir, ".claude/hooks") {
		t.Error(".claude/hooks/ left behind, empty")
	}
	if !exists(t, dir, ".claude/settings.local.json") {
		t.Error("the user's own file was removed")
	}
	taskfile, _ := os.ReadFile(filepath.Join(dir, "Taskfile.yml"))
	if bytes.Contains(taskfile, []byte("hook:guard")) {
		t.Error("Taskfile.yml still has the hook tasks")
	}
	if out, err := runAuditIn(t, dir); err != nil {
		t.Errorf("audit: %v\n%s", err, out)
	}
}

func TestIgnoredManagedPathIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDefaults)
	previous := checkIgnored
	checkIgnored = func(context.Context, string, []string) (map[string]bool, error) {
		return map[string]bool{".golangci.yml": true}, nil
	}
	t.Cleanup(func() { checkIgnored = previous })

	line := ".golangci.yml: error: ignored by git, so it would never be committed; add !.golangci.yml to .gitignore"
	if diff := runDiffIn(t, dir); !strings.Contains(diff, line) {
		t.Errorf("diff\n%s", diff)
	}
	if out, err := runAuditIn(t, dir); ExitCode(err) != exitNonConformant || !strings.Contains(out, line) {
		t.Errorf("audit exit %d\n%s", ExitCode(err), out)
	}
	out, err := runSyncIn(t, dir)
	if err == nil || !strings.Contains(err.Error(), "ignored by git") {
		t.Errorf("sync error %v\n%s", err, out)
	}
	if exists(t, dir, ".golangci.yml") {
		t.Error("sync wrote an ignored file")
	}
	if !exists(t, dir, "Taskfile.yml") {
		t.Error("an ignored file stopped every other file from being written")
	}
}

func TestNoGitIsAWarning(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDefaults)
	previous := checkIgnored
	checkIgnored = func(context.Context, string, []string) (map[string]bool, error) { return nil, errNoGit }
	t.Cleanup(func() { checkIgnored = previous })

	stubHookInstall(t)
	_, errOut, err := runSyncCapturing(t, dir)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n := strings.Count(errOut, "not checked whether git ignores managed files"); n != 1 {
		t.Errorf("warning printed %d times, want 1\n%s", n, errOut)
	}
}

// TestGitCheckIgnored runs the real check against a real work tree.
func TestGitCheckIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := gitCheckIgnored(t.Context(), t.TempDir(), []string{"a"}); !errors.Is(err, errNoGit) {
		t.Errorf("outside a work tree: %v, want errNoGit", err)
	}

	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".vscode/*\n!.vscode/tasks.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := gitCheckIgnored(t.Context(), dir, []string{".vscode/tasks.json", ".vscode/extensions.json", "Taskfile.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[".vscode/extensions.json"] {
		t.Errorf("ignored %v, want only .vscode/extensions.json", got)
	}
	got, err = gitCheckIgnored(t.Context(), dir, []string{"Taskfile.yml"})
	if err != nil || len(got) != 0 {
		t.Errorf("nothing ignored: %v, %v", got, err)
	}
}
