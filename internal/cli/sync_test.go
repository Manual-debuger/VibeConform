package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/reconcile"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/standard"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// hookRecorder captures the repo roots hook registration was asked to
// install into, so tests can assert on registration without a real lefthook
// binary or a real git work tree.
type hookRecorder struct {
	roots []string
	err   error
}

// stubHookInstall replaces the lefthook seam for the duration of one test.
// Every sync test needs it, not just the ones about hooks: the suite syncs
// into t.TempDir(), which is not a git work tree, so an unstubbed run would
// shell out to a lefthook install that cannot succeed.
func stubHookInstall(t *testing.T) *hookRecorder {
	t.Helper()
	rec := &hookRecorder{}
	previous := installGitHooks
	installGitHooks = func(_ context.Context, repoRoot string) error {
		rec.roots = append(rec.roots, repoRoot)
		return rec.err
	}
	t.Cleanup(func() { installGitHooks = previous })
	return rec
}

// runSyncCapturing runs sync and returns stdout and stderr separately. It
// does not stub the hook seam; callers that care do it themselves.
func runSyncCapturing(t *testing.T, dir string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd("test")
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"sync", "--repo-root", dir})

	err = root.Execute()
	return out.String(), errOut.String(), err
}

func runSyncIn(t *testing.T, dir string) (string, error) {
	t.Helper()
	stubHookInstall(t)
	out, _, err := runSyncCapturing(t, dir)
	return out, err
}

// recordState updates one entry in an existing state file, leaving the rest
// alone, so a test can drift a single resource without disturbing the others.
func recordState(t *testing.T, dir, path, sha string) {
	t.Helper()
	s, err := state.Load(dir)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	s.Resources[path] = state.ResourceState{SHA256: sha}
	if err := state.Save(dir, s); err != nil {
		t.Fatalf("seeding state: %v", err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestSyncCmdCreatesResourceAndRecordsState(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	want := goToolingContent(t)

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("sync returned error: %v\n%s", err, out)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".golangci.yml"))
	if err != nil {
		t.Fatalf("reading written resource: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("written .golangci.yml does not match the module's resolved content")
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if recorded := s.Resources[".golangci.yml"].SHA256; recorded != sha256Hex(want) {
		t.Errorf("recorded hash = %q, want %q", recorded, sha256Hex(want))
	}

	// The created count grows as modules are added; what this test pins is
	// that this resource was created and nothing was updated or conflicted.
	for _, wantLine := range []string{
		"standard: prod-go/v1",
		".golangci.yml: created",
		"0 updated, 0 unchanged, 0 conflicts",
	} {
		if !bytes.Contains([]byte(out), []byte(wantLine)) {
			t.Errorf("sync output missing %q\n%s", wantLine, out)
		}
	}
}

// TestSyncCmdCreatesNestedPaths is the test spec 0008 deferred until a module
// produced a resource in a subdirectory.
func TestSyncCmdCreatesNestedPaths(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)

	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("sync returned error: %v", err)
	}

	nested := filepath.Join(dir, ".github", "workflows", "ci.yml")
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("sync did not create the resource or its parent directories: %v", err)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	// The key must be slash-separated regardless of host platform, or a
	// Windows run and a Unix run record different state for the same repo.
	if _, ok := s.Resources[".github/workflows/ci.yml"]; !ok {
		t.Errorf("state is missing the slash-separated key for the nested resource: %v", s.Resources)
	}
}

// TestSyncCmdAppliesResourceModes checks the reason ADR 0006 exists: hook
// scripts must land executable, or the guardrail they implement silently
// does not run. Ordinary config lands 0644, not the owner-only 0600 spec
// 0008 originally hardcoded.
func TestSyncCmdAppliesResourceModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not model Unix permission bits; this would test the platform, not sync")
	}

	dir := t.TempDir()
	writeManifest(t, dir)
	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("sync returned error: %v", err)
	}

	for path, want := range map[string]os.FileMode{
		".claude/hooks/block-dangerous.sh": 0o755,
		".golangci.yml":                    0o644,
	} {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %v, want %v", path, got, want)
		}
	}
}

func TestSyncCmdIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)

	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("first sync returned error: %v", err)
	}
	firstState, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
	if err != nil {
		t.Fatalf("reading state after first sync: %v", err)
	}

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("second sync returned error: %v\n%s", err, out)
	}

	if want := ".golangci.yml: unchanged"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
	if want := "0 created, 0 updated,"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q — a second run must write nothing\n%s", want, out)
	}
	if want := "0 conflicts"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}

	secondState, err := os.ReadFile(filepath.Join(dir, ".vibe", "state.yaml"))
	if err != nil {
		t.Fatalf("reading state after second sync: %v", err)
	}
	if !bytes.Equal(firstState, secondState) {
		t.Errorf("state changed across identical runs:\nfirst:\n%s\nsecond:\n%s", firstState, secondState)
	}
}

func TestSyncCmdRefusesToOverwriteConflict(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	handEdited := []byte("hand-authored: true\n")
	resourceFile := filepath.Join(dir, ".golangci.yml")
	if err := os.WriteFile(resourceFile, handEdited, 0o600); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}

	out, err := runSyncIn(t, dir)
	if err == nil {
		t.Fatalf("expected a non-zero exit for a conflict, got nil\n%s", out)
	}

	got, readErr := os.ReadFile(resourceFile)
	if readErr != nil {
		t.Fatalf("reading conflicted file: %v", readErr)
	}
	if !bytes.Equal(got, handEdited) {
		t.Error("sync overwrote a conflicted file; it must be left exactly as found")
	}

	if want := ".golangci.yml: conflict"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
}

func TestSyncCmdOverwritesDriftFromRecordedState(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	target := goToolingContent(t)

	// Sync everything first, so only the one resource under test drifts.
	if _, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("seeding sync: %v", err)
	}

	// The file on disk still matches what was last applied, while the
	// module's target has moved on: reconcile's safe-replacement row.
	stale := []byte("previously-applied: true\n")
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), stale, 0o600); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}
	recordState(t, dir, ".golangci.yml", sha256Hex(stale))

	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("sync returned error: %v\n%s", err, out)
	}

	got, err := os.ReadFile(filepath.Join(dir, ".golangci.yml"))
	if err != nil {
		t.Fatalf("reading written resource: %v", err)
	}
	if !bytes.Equal(got, target) {
		t.Error("sync did not replace a stale file with the module's target content")
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatalf("loading state: %v", err)
	}
	if recorded := s.Resources[".golangci.yml"].SHA256; recorded != sha256Hex(target) {
		t.Errorf("recorded hash = %q, want the target's %q", recorded, sha256Hex(target))
	}

	if want := ".golangci.yml: updated"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
	if want := "0 created, 1 updated,"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q — exactly one resource should have been rewritten\n%s", want, out)
	}
	if want := "0 conflicts"; !bytes.Contains([]byte(out), []byte(want)) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
}

// TestSyncCmdRegistersGitHooks covers the reason spec 0014's first
// increment exists: prod-go/v1 manages lefthook.yml, and a repository
// that has the config without the hooks has a pre-commit gate that is
// configured and off.
func TestSyncCmdRegistersGitHooks(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	rec := stubHookInstall(t)

	out, _, err := runSyncCapturing(t, dir)
	if err != nil {
		t.Fatalf("sync returned error: %v\n%s", err, out)
	}

	if len(rec.roots) != 1 {
		t.Fatalf("hook registration ran %d time(s), want exactly 1", len(rec.roots))
	}
	if rec.roots[0] != dir {
		t.Errorf("registered hooks in %q, want the synced repo root %q", rec.roots[0], dir)
	}
	if want := "lefthook: git hooks registered"; !strings.Contains(out, want) {
		t.Errorf("sync output missing %q\n%s", want, out)
	}
}

// noLefthookModule is a fixture module for
// TestSyncCmdSkipsHookRegistrationForStandardsWithoutLefthook: as of spec
// 0016, every real registered standard manages lefthook.yml, so the
// negative case needs a standard of its own to exercise the sync command's
// wiring end to end (the unit-level case already lives in
// TestPlanManagesLefthook).
type noLefthookModule struct{}

func (noLefthookModule) Name() string { return "no-lefthook-fixture" }

func (noLefthookModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{Path: "NOTES.md", Ownership: resource.Generated, Content: []byte("fixture\n")},
	}, nil
}

func init() {
	// Named without the substring "lefthook": this test asserts sync output
	// never mentions that word, and the standard name is itself part of
	// that output ("standard: <name>/v1").
	standard.Register(standard.Standard{
		Name:    "test-fixture-no-hooks",
		Version: "v1",
		Modules: []module.Module{noLefthookModule{}},
	})
}

// TestSyncCmdSkipsHookRegistrationForStandardsWithoutLefthook is the
// integration counterpart to TestPlanManagesLefthook's negative case: a
// standard that manages no lefthook.yml must not have its git hooks
// touched.
func TestSyncCmdSkipsHookRegistrationForStandardsWithoutLefthook(t *testing.T) {
	dir := t.TempDir()
	writeManifestFor(t, dir, "test-fixture-no-hooks", "v1")
	rec := stubHookInstall(t)

	out, _, err := runSyncCapturing(t, dir)
	if err != nil {
		t.Fatalf("sync returned error: %v\n%s", err, out)
	}

	if len(rec.roots) != 0 {
		t.Errorf("registered git hooks for a standard that manages no lefthook.yml: %v", rec.roots)
	}
	if strings.Contains(out, "lefthook") {
		t.Errorf("sync output mentions lefthook for a standard that does not manage it\n%s", out)
	}
}

// TestSyncCmdSkipsHookRegistrationOnConflict pins the conservative half of
// the rule: a run that refused to write part of the standard has not
// finished configuring the repository.
func TestSyncCmdSkipsHookRegistrationOnConflict(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte("hand-authored: true\n"), 0o600); err != nil {
		t.Fatalf("seeding .golangci.yml: %v", err)
	}
	rec := stubHookInstall(t)

	if _, _, err := runSyncCapturing(t, dir); err == nil {
		t.Fatal("expected a non-zero exit for a conflict, got nil")
	}

	if len(rec.roots) != 0 {
		t.Errorf("hook registration ran on a conflicted sync: %v", rec.roots)
	}
}

// TestSyncCmdSurvivesHookRegistrationFailure is the invariant the whole
// increment rests on: registering hooks is a convenience on top of a sync
// that already succeeded, so no failure of it may change the exit code.
func TestSyncCmdSurvivesHookRegistrationFailure(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	rec := stubHookInstall(t)
	rec.err = errors.New("not a git repository")

	out, errOut, err := runSyncCapturing(t, dir)
	if err != nil {
		t.Fatalf("a failed hook registration must not fail the sync: %v\n%s", err, out)
	}

	if want := "warning: git hooks were not registered"; !strings.Contains(errOut, want) {
		t.Errorf("stderr missing %q\n%s", want, errOut)
	}
	if !strings.Contains(errOut, "not a git repository") {
		t.Errorf("stderr should carry lefthook's own reason\n%s", errOut)
	}
	if strings.Contains(out, "warning") {
		t.Errorf("warnings belong on stderr, not in sync's report of what it did\n%s", out)
	}
}

func TestPlanManagesLefthook(t *testing.T) {
	lefthook := resourcePlan{
		Resource:  resource.Resource{Path: lefthookResourcePath, Ownership: resource.Generated},
		Decision:  reconcile.NoChange,
		Supported: true,
	}
	other := resourcePlan{
		Resource:  resource.Resource{Path: ".golangci.yml", Ownership: resource.Generated},
		Decision:  reconcile.NoChange,
		Supported: true,
	}
	// An ownership mode no command handles yet is not a managed lefthook.yml:
	// sync would not have written it, so there is nothing to register.
	unsupported := resourcePlan{
		Resource: resource.Resource{Path: lefthookResourcePath, Ownership: resource.ProjectOwned},
	}

	for name, tc := range map[string]struct {
		resources []resourcePlan
		want      bool
	}{
		"manages lefthook":      {[]resourcePlan{other, lefthook}, true},
		"does not":              {[]resourcePlan{other}, false},
		"no resources at all":   {nil, false},
		"unsupported ownership": {[]resourcePlan{unsupported}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := planManagesLefthook(&repoPlan{Resources: tc.resources}); got != tc.want {
				t.Errorf("planManagesLefthook() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTrimOutput uses lefthook's actual failure output: it boxes a command
// echo above the line that says what went wrong, and pads every line out to
// a fixed width, so neither the first line nor the last is the useful one.
func TestTrimOutput(t *testing.T) {
	lefthookFailure := "│  > git rev-parse --show-toplevel                                  \n" +
		"│    fatal: not a git repository (or any of the parent directories): .git   \n" +
		"│                                                                          \n" +
		"exit status 128\n"

	got := trimOutput([]byte(lefthookFailure), "exit status 128")
	if strings.Contains(got, "  \n") || strings.HasSuffix(got, " ") {
		t.Errorf("trailing padding survived:\n%q", got)
	}
	if !strings.Contains(got, "fatal: not a git repository") {
		t.Errorf("dropped the line that says what went wrong:\n%s", got)
	}
	// The box-drawing line and the repeated exit status both cost a line and
	// say nothing; the warning already leads with the exit status.
	for _, unwanted := range []string{"│\n", "exit status 128"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("kept %q, which carries no information:\n%s", unwanted, got)
		}
	}

	var long strings.Builder
	for i := range maxOutputLines + 5 {
		fmt.Fprintf(&long, "line %d\n", i)
	}
	capped := strings.Split(trimOutput([]byte(long.String()), ""), "\n")
	if len(capped) != maxOutputLines+1 {
		t.Errorf("kept %d lines, want %d plus an ellipsis", len(capped), maxOutputLines)
	}
	if last := capped[len(capped)-1]; last != "..." {
		t.Errorf("truncated output should end in an ellipsis, got %q", last)
	}
}

// TestRunLefthookInstallReportsMissingBinary exercises the real
// implementation rather than the seam, since the seam is what every other
// test replaces.
func TestRunLefthookInstallReportsMissingBinary(t *testing.T) {
	t.Setenv("PATH", "")

	err := runLefthookInstall(context.Background(), t.TempDir())
	if !errors.Is(err, errLefthookNotFound) {
		t.Errorf("err = %v, want errLefthookNotFound", err)
	}
}

func TestSyncCmdFailsIfManifestMissing(t *testing.T) {
	dir := t.TempDir()

	if _, err := runSyncIn(t, dir); err == nil {
		t.Fatal("expected error when vibe.yaml is missing, got nil")
	}

	if _, err := os.Stat(filepath.Join(dir, ".vibe")); !os.IsNotExist(err) {
		t.Errorf("sync should not create .vibe/ when it cannot read the manifest, stat err = %v", err)
	}
}

func TestSyncCmdFailsIfStandardUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte("standard: prod-go\nversion: v99\n"), 0o600); err != nil {
		t.Fatalf("seeding vibe.yaml: %v", err)
	}

	if _, err := runSyncIn(t, dir); err == nil {
		t.Fatal("expected error for unknown standard/version, got nil")
	}
}
