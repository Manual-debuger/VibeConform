package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/standard"
	"github.com/Manual-debuger/VibeConform/internal/state"
)

// seedFixture resolves a base config and the seeded file that extends
// it, in a component directory, as prod-mono re-roots them (spec 0049).
type seedFixture struct{}

const (
	fixtureBase = "comp/lint.base.cfg"
	fixtureSeed = "comp/lint.cfg"
	seedContent = "extend = \"lint.base.cfg\"\n"
	baseContent = "rules = [\"all\"]\n"
)

func (seedFixture) Name() string { return "seed-fixture" }

func (seedFixture) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return []resource.Resource{
		{Path: fixtureBase, Ownership: resource.Generated, Content: []byte(baseContent)},
		{Path: fixtureSeed, Ownership: resource.Seeded, Content: []byte(seedContent), Base: "lint.base.cfg"},
	}, nil
}

func init() {
	standard.Register(standard.Standard{Name: "test-fixture-seed", Version: "v1", Modules: []module.Module{seedFixture{}}})
}

func seedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeManifestFor(t, dir, "test-fixture-seed", "v1")
	return dir
}

func mustContain(t *testing.T, what, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, out)
		}
	}
}

func noSeedRecord(t *testing.T, dir string) {
	t.Helper()
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Resources[fixtureSeed]; ok {
		t.Errorf("state records the seeded %s", fixtureSeed)
	}
}

// TestSeedCreatedOnce pins D3, D4, D6 and D7: a missing seeded file is
// drifted and created, never recorded, never written once it exists
// whatever it holds, and created again when deleted.
func TestSeedCreatedOnce(t *testing.T) {
	dir := seedRepo(t)
	out, err := runAuditIn(t, dir)
	if ExitCode(err) == 0 {
		t.Fatalf("audit passed with the seeded file missing:\n%s", out)
	}
	mustContain(t, "audit", out, fixtureSeed+": missing (run vibe sync to create it)")
	mustContain(t, "diff", runDiffIn(t, dir), fixtureSeed+": create (no file on disk; project-owned once written)")

	mustContain(t, "sync", mustSync(t, dir), fixtureSeed+": created (project-owned from now on)")
	if got := readFile(t, dir, fixtureSeed); got != seedContent {
		t.Errorf("seed = %q", got)
	}
	noSeedRecord(t, dir)
	out, err = runAuditIn(t, dir)
	if err != nil {
		t.Fatalf("audit after sync: %v\n%s", err, out)
	}
	mustContain(t, "audit", out, fixtureSeed+": ok (project-owned)")
	mustContain(t, "second sync", mustSync(t, dir), fixtureSeed+": unchanged (project-owned)", "0 created, 0 updated")

	extended := seedContent + "rules.off = [\"one\"]\n"
	writeFile(t, dir, fixtureSeed, extended)
	out, err = runAuditIn(t, dir)
	if err != nil {
		t.Fatalf("audit of an extended file failed: %v\n%s", err, out)
	}
	mustContain(t, "audit", out, fixtureSeed+": extended (project-owned; differs from the seed)", "conformant")
	mustSync(t, dir)
	if got := readFile(t, dir, fixtureSeed); got != extended {
		t.Errorf("sync wrote the repository's file: %q", got)
	}

	if err := removeFile(dir, fixtureSeed); err != nil {
		t.Fatal(err)
	}
	mustSync(t, dir)
	if got := readFile(t, dir, fixtureSeed); got != seedContent {
		t.Errorf("deleted seed not recreated: %q", got)
	}
}

// TestSeedWithoutBase pins D5: a file that no longer names its base is a
// conflict in every command, and sync leaves it alone.
func TestSeedWithoutBase(t *testing.T) {
	dir := seedRepo(t)
	mustSync(t, dir)
	standalone := "rules = []\n"
	writeFile(t, dir, fixtureSeed, standalone)

	want := fixtureSeed + ": conflict: does not name lint.base.cfg"
	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("audit exit %d, want 2:\n%s", code, out)
	}
	mustContain(t, "audit", out, want, "1 conflicts")
	mustContain(t, "diff", runDiffIn(t, dir), want)
	out, err = runSyncIn(t, dir)
	if err == nil {
		t.Fatalf("sync passed:\n%s", out)
	}
	mustContain(t, "sync", out, want)
	if got := readFile(t, dir, fixtureSeed); got != standalone {
		t.Errorf("sync wrote the file: %q", got)
	}
}

// managedBefore makes dir look synced by a version that owned the whole
// seeded file: content on disk, its hash recorded as generated.
func managedBefore(t *testing.T, content string) string {
	t.Helper()
	dir := seedRepo(t)
	mustSync(t, dir)
	writeFile(t, dir, fixtureSeed, content)
	recordState(t, dir, fixtureSeed, sha256Hex([]byte(content)))
	return dir
}

// TestSeedConvertsUnchangedManagedFile pins D9.
func TestSeedConvertsUnchangedManagedFile(t *testing.T) {
	dir := managedBefore(t, baseContent)
	out, err := runAuditIn(t, dir)
	if ExitCode(err) == 0 {
		t.Fatalf("audit passed before the conversion:\n%s", out)
	}
	mustContain(t, "audit", out, fixtureSeed+": out of date (now project-owned: run vibe sync to replace it with a file extending lint.base.cfg)")
	mustContain(t, "diff", runDiffIn(t, dir), fixtureSeed+": would replace with a file extending lint.base.cfg")
	mustContain(t, "sync", mustSync(t, dir), fixtureSeed+": converted: now extends lint.base.cfg (project-owned)")
	if got := readFile(t, dir, fixtureSeed); got != seedContent {
		t.Errorf("converted file = %q", got)
	}
	noSeedRecord(t, dir)
	mustConform(t, dir)
}

// TestSeedEditedManagedFile pins D10: an edited managed file is a
// conflict, the base is held, and naming the base resolves it.
func TestSeedEditedManagedFile(t *testing.T) {
	dir := managedBefore(t, baseContent)
	if err := removeFile(dir, fixtureBase); err != nil {
		t.Fatal(err)
	}
	edited := baseContent + "rules.off = [\"one\"]\n"
	writeFile(t, dir, fixtureSeed, edited)

	out, err := runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("audit exit %d, want 2:\n%s", code, out)
	}
	conflict := fixtureSeed + ": conflict: edited since vibe last wrote it; it is now yours and must name lint.base.cfg"
	mustContain(t, "audit", out, conflict, `docs/usage.md, "Base configs"`, fixtureBase+": missing (run vibe sync); held until "+fixtureSeed+" is resolved")
	mustContain(t, "diff", runDiffIn(t, dir), conflict, fixtureBase+": held: not written until "+fixtureSeed+" is resolved")
	out, err = runSyncIn(t, dir)
	if err == nil {
		t.Fatalf("sync passed:\n%s", out)
	}
	mustContain(t, "sync", out, conflict, fixtureBase+": held")
	if exists(t, dir, fixtureBase) {
		t.Error("sync wrote the base next to an unresolved standalone config")
	}
	if got := readFile(t, dir, fixtureSeed); got != edited {
		t.Errorf("sync wrote the edited file: %q", got)
	}

	// Resolved: the edits now sit after the base.
	resolved := seedContent + "rules.off = [\"one\"]\n"
	writeFile(t, dir, fixtureSeed, resolved)
	mustContain(t, "diff", runDiffIn(t, dir), fixtureSeed+": would stop tracking (extends lint.base.cfg; project-owned)")
	mustContain(t, "sync", mustSync(t, dir), fixtureSeed+": no longer tracked", fixtureBase+": created")
	noSeedRecord(t, dir)
	mustConform(t, dir)
	if got := readFile(t, dir, fixtureSeed); got != resolved {
		t.Errorf("sync wrote the resolved file: %q", got)
	}
}

// TestSeedGitignored pins D8.
func TestSeedGitignored(t *testing.T) {
	dir := seedRepo(t)
	previous := checkIgnored
	checkIgnored = func(context.Context, string, []string) (map[string]bool, error) {
		return map[string]bool{fixtureSeed: true}, nil
	}
	t.Cleanup(func() { checkIgnored = previous })
	out, err := runSyncIn(t, dir)
	if err == nil {
		t.Fatalf("sync passed:\n%s", out)
	}
	if exists(t, dir, fixtureSeed) {
		t.Error("sync wrote a gitignored seed")
	}
	out, err = runAuditIn(t, dir)
	if code := ExitCode(err); code != 2 {
		t.Fatalf("audit exit %d, want 2:\n%s", code, out)
	}
	mustContain(t, "audit", out, fixtureSeed+": error: ignored by git")
}

// TestPlanResourceRefusesMismatchedBase: Base belongs to Seeded alone.
func TestPlanResourceRefusesMismatchedBase(t *testing.T) {
	t.Parallel()
	for _, r := range []resource.Resource{
		{Path: "a", Ownership: resource.Seeded, Content: []byte("x")},
		{Path: "a", Ownership: resource.Generated, Content: []byte("x"), Base: "b"},
	} {
		if _, err := planResource(t.TempDir(), &state.State{}, seedFixture{}, r); err == nil {
			t.Errorf("planResource accepted %+v", r)
		}
	}
}

func removeFile(dir, rel string) error {
	return os.Remove(filepath.Join(dir, filepath.FromSlash(rel)))
}
