package standard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// lefthookExtends is the block spec 0036 adds to every lefthook template.
const lefthookExtends = `# Repository-specific hooks go in lefthook.local.yml (optional, yours;
# see VibeConform's docs/usage.md). Lefthook merges it over this file.
extends:
  - lefthook.local.yml

`

// lefthookHeader is the template header line spec 0036 made
// provider-neutral; lefthookHeaderBefore is what it replaced.
const (
	lefthookHeader       = "# CI is the authoritative full verification gate;\n"
	lefthookHeaderBefore = "# CI (.github/workflows/ci.yml) is the authoritative full verification gate;\n"
)

// lefthookCases resolves lefthook.yml for every standard with graphify
// unselected and selected, keyed by a name usable as a file name.
func lefthookCases(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for k, s := range registry {
		for _, graph := range []bool{false, true} {
			selected := slices.Clone(s.Defaults().Integrations)
			name := k.name + "-" + k.version
			if graph {
				if len(s.Options) == 0 {
					continue
				}
				selected = append(selected, "graphify")
				name += "-graphify"
			}
			r, ok := resolveAll(t, s, selected)["lefthook.yml"]
			if !ok {
				continue
			}
			out[name] = r.Content
		}
	}
	return out
}

// TestLefthookExtendsLocal pins spec 0036 acceptance criterion 1.
func TestLefthookExtendsLocal(t *testing.T) {
	cases := lefthookCases(t)
	if len(cases) == 0 {
		t.Fatal("no standard resolves lefthook.yml")
	}
	for name, content := range cases {
		var doc struct {
			Extends []string `yaml:"extends"`
		}
		if err := yaml.Unmarshal(content, &doc); err != nil {
			t.Fatalf("%s: parsing lefthook.yml: %v", name, err)
		}
		if !slices.Equal(doc.Extends, []string{"lefthook.local.yml"}) {
			t.Errorf("%s: extends = %q, want [lefthook.local.yml]", name, doc.Extends)
		}
		if strings.Contains(string(content), ".github/") {
			t.Errorf("%s: lefthook.yml names a .github/ path", name)
		}
	}
}

// TestLocalLefthookIsNotManaged pins spec 0036 acceptance criterion 2:
// lefthook.local.yml is the project's, so no standard resolves it and no
// Go code outside tests reads or writes it. Only the templates name it,
// and the vibeconform skill's prose (spec 0041), which tells an agent the
// file is the project's.
func TestLocalLefthookIsNotManaged(t *testing.T) {
	prose := filepath.Join("..", "module", "agents", "vibeskill", "vibeskill.go")
	for k, s := range registry {
		selected := slices.Clone(s.Defaults().Integrations)
		if len(s.Options) > 0 {
			selected = append(selected, "graphify")
		}
		for path := range resolveAll(t, s, selected) {
			if path == "lefthook.local.yml" {
				t.Errorf("%s/%s resolves lefthook.local.yml", k.name, k.version)
			}
		}
	}
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == prose {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "lefthook.local.yml") {
			t.Errorf("%s names lefthook.local.yml", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLefthookOnlyGainsExtends pins spec 0036 acceptance criterion 3:
// apart from the extends block and the provider-neutral header line, every
// resolved lefthook.yml is byte-identical to its resolution before the
// change, recorded under testdata/lefthook-0036/. VIBE_WRITE_GOLDEN=1
// rewrites the goldens from the current output with the change undone.
func TestLefthookOnlyGainsExtends(t *testing.T) {
	dir := filepath.Join("testdata", "lefthook-0036")
	for name, content := range lefthookCases(t) {
		got := string(content)
		write := os.Getenv("VIBE_WRITE_GOLDEN") == "1"
		if strings.Count(got, lefthookExtends) != 1 && !write {
			t.Errorf("%s: extends block not present exactly once", name)
			continue
		}
		got = strings.Replace(got, lefthookExtends, "", 1)
		got = strings.Replace(got, lefthookHeader, lefthookHeaderBefore, 1)
		golden := filepath.Join(dir, name+".yml")
		if write {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != string(want) {
			t.Errorf("%s: lefthook.yml changed beyond the extends block and header:\n--- got\n%s\n--- want\n%s", name, got, want)
		}
	}
}
