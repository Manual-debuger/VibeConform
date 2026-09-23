package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/state"
)

// runAs runs one command with an explicit binary version, which is what
// state provenance is compared against. The rest of the suite uses
// NewRootCmd("test"); "test" is not valid semver, so it is deliberately
// unorderable and these guards stay dormant there.
func runAs(t *testing.T, version string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd(version)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	return func() (string, error) { err := root.Execute(); return out.String(), err }()
}

// seedSyncedRepo produces a repository synced by the named vibe version,
// with one resource whose target has since moved on — the shape every guard
// below reasons about.
func seedSyncedRepo(t *testing.T, writer string) string {
	t.Helper()
	dir := t.TempDir()
	writeManifest(t, dir)
	stubHookInstall(t)
	if _, err := runAs(t, writer, "sync", "--repo-root", dir); err != nil {
		t.Fatalf("seeding sync: %v", err)
	}

	// The repository is untouched and the standard moved: C == P, T != P.
	stale := []byte("previously-applied: true\n")
	if err := os.WriteFile(filepath.Join(dir, ".golangci.yml"), stale, 0o600); err != nil {
		t.Fatalf("staling resource: %v", err)
	}
	recordState(t, dir, ".golangci.yml", sha256Hex(stale))
	return dir
}

// TestAuditGivesNoVerdictWithStaleBinary is the guard for the case that
// motivated spec 0019. A binary older than the one that wrote the state has
// older embedded templates, so it reports an untouched repository as
// needing a sync that would in fact revert it. It must decline to judge
// rather than judge wrongly.
func TestAuditGivesNoVerdictWithStaleBinary(t *testing.T) {
	dir := seedSyncedRepo(t, "v0.3.0")

	out, err := runAs(t, "v0.2.0", "audit", "--repo-root", dir)

	if code := ExitCode(err); code != 1 {
		t.Fatalf("ExitCode = %d, want 1 — a stale binary cannot reach a verdict\n%s", code, out)
	}
	if !strings.Contains(out, "no verdict") {
		t.Errorf("output does not decline to judge:\n%s", out)
	}
	for _, want := range []string{"v0.2.0", "v0.3.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not name %s:\n%s", want, out)
		}
	}
	// The repository may be in perfect shape; the install is what is behind.
	if strings.Contains(out, "not conformant") {
		t.Errorf("stale binary blamed the repository:\n%s", out)
	}
}

// TestSyncRefusesToRevertWithStaleBinary is the destructive half. Verified
// by hand against a real v0.2.0-alpha.1 binary, a sync in this situation
// reverted three specs' worth of managed content and then recorded the
// result as conformant.
func TestSyncRefusesToRevertWithStaleBinary(t *testing.T) {
	dir := seedSyncedRepo(t, "v0.3.0")
	before, err := os.ReadFile(filepath.Join(dir, ".golangci.yml"))
	if err != nil {
		t.Fatalf("reading resource: %v", err)
	}

	out, err := runAs(t, "v0.2.0", "sync", "--repo-root", dir)

	if err == nil {
		t.Fatalf("sync succeeded; it must refuse to revert\n%s", out)
	}
	if code := ExitCode(err); code != 1 {
		t.Errorf("ExitCode = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(err.Error(), "--allow-downgrade") {
		t.Errorf("refusal does not name the opt-in: %v", err)
	}

	after, readErr := os.ReadFile(filepath.Join(dir, ".golangci.yml"))
	if readErr != nil {
		t.Fatalf("reading resource: %v", readErr)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("sync wrote the file before refusing; the refusal must happen "+
			"before anything is written\nbefore: %q\nafter:  %q", before, after)
	}

	// The recorded writer must survive too: rewriting state to this older
	// binary would make the next run think the downgrade was intended.
	s, loadErr := state.Load(dir)
	if loadErr != nil {
		t.Fatalf("loading state: %v", loadErr)
	}
	if s.VibeVersion != "v0.3.0" {
		t.Errorf("recorded writer = %q, want v0.3.0 — a refused sync must not "+
			"rewrite provenance", s.VibeVersion)
	}
}

// TestSyncAllowDowngradeOverridesTheRefusal keeps the escape hatch honest:
// deliberately reverting is legitimate, and must be possible without
// editing state by hand.
func TestSyncAllowDowngradeOverridesTheRefusal(t *testing.T) {
	dir := seedSyncedRepo(t, "v0.3.0")

	out, err := runAs(t, "v0.2.0", "sync", "--repo-root", dir, "--allow-downgrade")
	if err != nil {
		t.Fatalf("sync with --allow-downgrade failed: %v\n%s", err, out)
	}

	s, loadErr := state.Load(dir)
	if loadErr != nil {
		t.Fatalf("loading state: %v", loadErr)
	}
	if s.VibeVersion != "v0.2.0" {
		t.Errorf("recorded writer = %q, want v0.2.0 — an accepted downgrade "+
			"records the binary that actually wrote the files", s.VibeVersion)
	}
}

// TestNewerBinarySyncsWithoutCeremony pins the ordinary upgrade path. The
// guard fires on older, not on different: an adopter whose vibe advanced
// must not be asked to confirm anything.
func TestNewerBinarySyncsWithoutCeremony(t *testing.T) {
	dir := seedSyncedRepo(t, "v0.2.0")

	out, err := runAs(t, "v0.3.0", "sync", "--repo-root", dir)
	if err != nil {
		t.Fatalf("newer binary must sync without an opt-in: %v\n%s", err, out)
	}
}

// TestUnorderableVersionsClaimNothing covers every repository that exists
// today: no provenance recorded, or an unstamped "dev" build. Neither may
// be treated as stale, or vibe would refuse to sync on the most common
// developer path there is.
func TestUnorderableVersionsClaimNothing(t *testing.T) {
	for _, tc := range []struct{ name, writer, running string }{
		{"no provenance recorded", "", "v0.3.0"},
		{"unstamped local build", "v0.3.0", "dev"},
		{"both unstamped", "dev", "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := seedSyncedRepo(t, tc.writer)
			if tc.writer == "" {
				// Strip provenance to reproduce a pre-0019 state file.
				s, err := state.Load(dir)
				if err != nil {
					t.Fatalf("loading state: %v", err)
				}
				s.Schema, s.VibeVersion, s.Standard = 0, "", ""
				if err := state.Save(dir, s); err != nil {
					t.Fatalf("seeding state: %v", err)
				}
			}

			out, err := runAs(t, tc.running, "sync", "--repo-root", dir)
			if err != nil {
				t.Fatalf("unorderable versions must not block a sync: %v\n%s", err, out)
			}
		})
	}
}
