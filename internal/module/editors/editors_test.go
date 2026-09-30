package editors_test

import (
	"context"
	"slices"
	"testing"

	"github.com/tailscale/hujson"

	"github.com/Manual-debuger/VibeConform/internal/jsonarray"
	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/editors"
	"github.com/Manual-debuger/VibeConform/internal/module/editors/vscode"
	"github.com/Manual-debuger/VibeConform/internal/module/editors/zed"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// TestElementsAreWellFormed: every owned element parses, is identified by
// its declared ID, and every skeleton parses with its array present and
// empty — the invariants the structured-patch planner relies on.
func TestElementsAreWellFormed(t *testing.T) {
	mctx := &module.Context{Profiles: manifest.Profiles}
	for _, m := range []module.Module{vscode.New(), zed.New()} {
		rs, err := m.Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatalf("%s: %v", m.Name(), err)
		}
		if len(rs) == 0 {
			t.Fatalf("%s resolves nothing", m.Name())
		}
		for _, r := range rs {
			if r.Ownership != resource.StructuredPatch || r.Patch == nil {
				t.Fatalf("%s: %s is not a structured patch", m.Name(), r.Path)
			}
			doc, err := jsonarray.Parse(r.Patch.Skeleton, r.Patch.Array)
			if err != nil || doc.Len() != 0 {
				t.Errorf("%s: skeleton %q: %v, %d elements", r.Path, r.Patch.Skeleton, err, doc.Len())
			}
			seen := map[string]bool{}
			for _, e := range r.Patch.Elements {
				v, err := hujson.Parse(e.Value)
				if err != nil {
					t.Errorf("%s: element %s: %v", r.Path, e.Value, err)
					continue
				}
				if id, ok := jsonarray.Identity(v); !ok || id != e.ID {
					t.Errorf("%s: element %s identifies as %q, declared %q", r.Path, e.Value, id, e.ID)
				}
				if seen[e.ID] {
					t.Errorf("%s: duplicate element %q", r.Path, e.ID)
				}
				seen[e.ID] = true
			}
		}
	}
}

// TestVSCodeRecommendsOnlyDeclaredProfiles: recommendations are per
// declared language, and no language means no extensions.json at all.
func TestVSCodeRecommendsOnlyDeclaredProfiles(t *testing.T) {
	for name, tc := range map[string]struct {
		profiles []manifest.Profile
		want     []string
	}{
		"go":   {[]manifest.Profile{manifest.ProfileGo}, []string{"golang.go"}},
		"py":   {[]manifest.Profile{manifest.ProfilePy}, []string{"charliermarsh.ruff", "ms-python.python"}},
		"none": {nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			rs, err := vscode.New().Resolve(context.Background(), &module.Context{Profiles: tc.profiles})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range rs {
				if r.Path != ".vscode/extensions.json" {
					continue
				}
				for _, e := range r.Patch.Elements {
					got = append(got, e.ID)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("recommendations %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("recommendations %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestOwnedPathsMatchResolvedPaths: OwnedPaths, which core modules use to
// keep formatters off editor files, names exactly the files each
// integration resolves with every profile declared.
func TestOwnedPathsMatchResolvedPaths(t *testing.T) {
	mctx := &module.Context{Profiles: manifest.Profiles}
	for name, m := range map[string]module.Module{"vscode": vscode.New(), "zed": zed.New()} {
		rs, err := m.Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got []string
		for _, r := range rs {
			got = append(got, r.Path)
		}
		if !slices.Equal(got, editors.OwnedPaths[name]) {
			t.Errorf("%s resolves %v, OwnedPaths says %v", name, got, editors.OwnedPaths[name])
		}
	}
	if len(editors.OwnedPaths) != 2 {
		t.Errorf("OwnedPaths has %d integrations; add the new one to this test", len(editors.OwnedPaths))
	}
}

func TestOwnedPathsOf(t *testing.T) {
	for _, tc := range []struct {
		selection []string
		want      []string
	}{
		{nil, nil},
		{[]string{"claude", "codex"}, nil},
		{[]string{"vscode", "claude"}, []string{".vscode/tasks.json", ".vscode/extensions.json"}},
		{[]string{"vscode", "zed"}, []string{".vscode/tasks.json", ".vscode/extensions.json", ".zed/tasks.json"}},
	} {
		if got := editors.OwnedPathsOf(tc.selection); !slices.Equal(got, tc.want) {
			t.Errorf("OwnedPathsOf(%v) = %v, want %v", tc.selection, got, tc.want)
		}
	}
}
