package setupskill_test

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// TestPlaceGivesTheRecordedPlaces: for every stack, provider and file of
// the skill, Place gives the place that the drift checker's
// referenceTarget gave on main before spec 0073 moved it here, or "not
// used". testdata/placements.tsv was recorded from that function at
// 2757b85: one line per used file, "stack provider ref place", in the
// order of its loop over referenceStacks, referenceProviders and
// referenceFiles. The same loop at e280d94, main before spec 0073, gives
// a byte-identical table.
func TestPlaceGivesTheRecordedPlaces(t *testing.T) {
	want := recordedPlaces(t)
	if got := setupskill.Stacks(); !slices.Equal(got, []string{"go", "python", "typescript", "monorepo"}) {
		t.Errorf("Stacks() = %v", got)
	}
	if got := setupskill.Providers(); !slices.Equal(got, []string{"github", "gitlab"}) {
		t.Errorf("Providers() = %v", got)
	}
	refs := skillFiles(t)
	seen := 0
	for _, stack := range setupskill.Stacks() {
		for _, provider := range setupskill.Providers() {
			for _, ref := range refs {
				got, ok := setupskill.Place(stack, provider, ref)
				w, wantOK := want[[3]string{stack, provider, ref}]
				if ok != wantOK || got != w {
					t.Errorf("Place(%s, %s, %s) = %q, %v; want %q, %v", stack, provider, ref, got, ok, w, wantOK)
				}
				if ok {
					seen++
				}
			}
		}
	}
	if seen != len(want) {
		t.Errorf("Place placed %d files; the table has %d", seen, len(want))
	}
}

// recordedPlaces reads testdata/placements.tsv: place by stack, provider
// and reference file.
func recordedPlaces(t *testing.T) map[[3]string]string {
	t.Helper()
	data, err := os.ReadFile("testdata/placements.tsv")
	if err != nil {
		t.Fatal(err)
	}
	want := map[[3]string]string{}
	for _, line := range strings.Split(strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			t.Fatalf("testdata/placements.tsv: %q has %d fields, want 4", line, len(f))
		}
		want[[3]string{f[0], f[1], f[2]}] = f[3]
	}
	return want
}

// skillFiles lists every file of the skill.
func skillFiles(t *testing.T) []string {
	t.Helper()
	var refs []string
	if err := fs.WalkDir(setupskill.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			refs = append(refs, p)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return refs
}

// TestPlacePutsThePolicyWhereTheGuardReadsIt: the reference policy goes
// to the file vibe-guard reads (internal/setupskill may not import
// internal/guardrule, so it names the file through internal/guard).
func TestPlacePutsThePolicyWhereTheGuardReadsIt(t *testing.T) {
	if got, ok := setupskill.Place("go", setupskill.ProviderGitHub, "reference/agents/policy.json"); !ok || got != guardrule.PolicyFile {
		t.Errorf("Place(policy.json) = %q, %v; want %q", got, ok, guardrule.PolicyFile)
	}
}

// TestMonorepoComponentsMatchTheReferences: the manifest's components
// are the components that reference/vibe-yaml/monorepo.yaml declares,
// the root reference Taskfile includes each under its ID from its
// directory, and each one's Taskfile is a reference file.
func TestMonorepoComponentsMatchTheReferences(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		data, err := fs.ReadFile(setupskill.FS(), name)
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	cfg := read("reference/vibe-yaml/monorepo.yaml")
	root := read("reference/taskrunner/monorepo/Taskfile.yml")
	comps := setupskill.MonorepoComponents()
	if len(comps) != 3 {
		t.Errorf("MonorepoComponents() has %d components, want 3", len(comps))
	}
	for _, c := range comps {
		if want := "  - path: " + c.Dir + "\n    language: " + c.Language + "\n"; !strings.Contains(cfg, want) {
			t.Errorf("monorepo.yaml does not declare %q", want)
		}
		if want := "  " + c.ID + ":\n    taskfile: ./" + c.Dir + "/Taskfile.yml\n    dir: ./" + c.Dir + "\n"; !strings.Contains(root, want) {
			t.Errorf("the root Taskfile does not include %q", want)
		}
		if _, err := fs.Stat(setupskill.FS(), c.Taskfile()); err != nil {
			t.Errorf("component %s: %v", c.ID, err)
		}
	}
}
