package standard

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

// resolveGenerated resolves s in order, with generated declared at the top
// level (single-language) or on every non-Go component (prod-mono).
func resolveGenerated(t *testing.T, s Standard, generated []string) []resource.Resource {
	t.Helper()
	mctx := sampleContext(s)
	if mctx == nil {
		mctx = &module.Context{Generated: generated}
	} else {
		for i := range mctx.Components {
			if mctx.Components[i].Profile != manifest.ProfileGo {
				mctx.Components[i].Generated = generated
			}
		}
	}
	mctx.Integrations = s.Defaults().Integrations
	var out []resource.Resource
	for _, m := range s.ModulesFor(Selection{Integrations: mctx.Integrations}) {
		rs, err := m.Resolve(context.Background(), mctx)
		if err != nil {
			t.Fatalf("resolving %s: %v", m.Name(), err)
		}
		out = append(out, rs...)
	}
	return out
}

// TestGeneratedAbsentIsByteIdentical pins spec 0037 acceptance criterion 1:
// an absent and an empty generated: list resolve exactly what every
// standard resolved before the key existed, in the same order.
func TestGeneratedAbsentIsByteIdentical(t *testing.T) {
	for k, s := range registry {
		t.Run(k.name+"/"+k.version, func(t *testing.T) {
			base := resolveGenerated(t, s, nil)
			got := resolveGenerated(t, s, []string{})
			if len(got) != len(base) {
				t.Fatalf("%d resources, want %d", len(got), len(base))
			}
			for i := range base {
				if got[i].Path != base[i].Path || string(got[i].Content) != string(base[i].Content) {
					t.Errorf("resource %d: %s differs from %s", i, got[i].Path, base[i].Path)
				}
			}
		})
	}
}

// TestGeneratedRendering pins spec 0037 acceptance criteria 4 and 5: with
// generated declared, only the §4 files change, by exactly the §4 lines,
// in declaration order; type-checking configuration does not change.
func TestGeneratedRendering(t *testing.T) {
	generated := []string{"src/contracts/**", "src/api.gen.ts"}
	allowed := map[string][]string{
		"prod-py": {"ruff.toml"},
		"prod-ts": {"eslint.config.js", "lefthook.yml", ".prettierignore"},
		"prod-mono": {
			"services/worker/ruff.toml",
			"apps/web/eslint.config.js", "apps/web/.prettierignore",
		},
	}
	for k, s := range registry {
		want, ok := allowed[k.name]
		if !ok {
			continue
		}
		t.Run(k.name+"/"+k.version, func(t *testing.T) {
			base := map[string]string{}
			for _, r := range resolveGenerated(t, s, nil) {
				base[r.Path] = string(r.Content)
			}
			got := map[string]string{}
			for _, r := range resolveGenerated(t, s, generated) {
				got[r.Path] = string(r.Content)
			}
			for path, content := range got {
				before, existed := base[path]
				changed := !existed || before != content
				if changed != slices.Contains(want, path) {
					t.Errorf("%s changed = %v, want %v", path, changed, !changed)
				}
			}
			for path, content := range got {
				switch {
				case strings.HasSuffix(path, "ruff.toml"):
					add := "\n# Generated code (vibe.yaml generated:): not formatted or linted, still\n" +
						"# type-checked. force-exclude applies it to files passed explicitly too.\n" +
						`extend-exclude = ["src/contracts/**", "src/api.gen.ts"]` + "\n" +
						"force-exclude = true\n"
					if strings.Replace(content, add, "", 1) != base[path] {
						t.Errorf("%s differs by more than the generated keys:\n%s", path, content)
					}
					// Both keys must be top-level: before the first table.
					table := strings.Index(content, "\n[")
					if i := strings.Index(content, "force-exclude"); i < 0 || i > table {
						t.Errorf("%s: force-exclude is not a top-level key", path)
					}
				case strings.HasSuffix(path, "eslint.config.js"):
					line := "    ignores: ['**/dist/**', '**/coverage/**', 'src/contracts/**', 'src/api.gen.ts'],\n"
					if !strings.Contains(content, line) {
						t.Errorf("%s lacks %q", path, line)
					}
					if strings.Replace(content, ", 'src/contracts/**', 'src/api.gen.ts'", "", 1) != base[path] {
						t.Errorf("%s differs by more than the appended ignores:\n%s", path, content)
					}
				case strings.HasSuffix(path, ".prettierignore"):
					wantSection := "# Managed by VibeConform: generated: in vibe.yaml. Not formatted.\nsrc/contracts/**\nsrc/api.gen.ts\n"
					if content != wantSection {
						t.Errorf("%s section = %q, want %q", path, content, wantSection)
					}
				case path == "lefthook.yml" && k.name == "prod-ts":
					if strings.Replace(content, "eslint --no-warn-ignored {staged_files}", "eslint {staged_files}", 1) != base[path] {
						t.Errorf("lefthook.yml differs by more than --no-warn-ignored:\n%s", content)
					}
				}
			}
		})
	}
}
