package pins_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/github"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/githubmono"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/githubpy"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/githubts"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/gitlabmono"
	"github.com/Manual-debuger/VibeConform/internal/module/ci/pins"
	"github.com/Manual-debuger/VibeConform/internal/module/conformance"
)

// fields returns the table's entries by field name.
func fields(t pins.Table) map[string]string {
	v := reflect.ValueOf(t)
	out := map[string]string{}
	for i := range v.NumField() {
		out[v.Type().Field(i).Name] = v.Field(i).String()
	}
	return out
}

// TestTemplatesHoldNoPins: no CI or conformance template names a pinned
// version itself, so the table is the only place a bump has to happen
// (spec 0044 §1). A value counts as named when it appears quoted, after @
// (go install), or after : (an image tag).
func TestTemplatesHoldNoPins(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join("..", "*", "templates", "*"))
	if err != nil {
		t.Fatal(err)
	}
	more, err := filepath.Glob(filepath.Join("..", "..", "conformance", "templates", "*"))
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, more...)
	if len(sources) < 10 {
		t.Fatalf("found only %d template sources: %v", len(sources), sources)
	}
	for _, path := range sources {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range fields(pins.Current) {
			for _, literal := range []string{`"` + value + `"`, "@" + value, ":" + value} {
				if bytes.Contains(content, []byte(literal)) {
					t.Errorf("%s names %s (%s) as %s; use [[.Pins.%s]]", path, name, value, literal, name)
				}
			}
		}
	}
}

var components = []manifest.Component{
	{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
	{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
	{ID: "worker", Path: "services/worker", Profile: manifest.ProfilePy},
}

// renderAll resolves every module that writes a CI file, for every standard
// and provider that writes one, and returns the CI files by case and path.
func renderAll(t *testing.T) map[string][]byte {
	t.Helper()
	gitlab := &module.Context{Components: components, Policies: map[string]string{manifest.CIProvider: module.CIGitLab}}
	cases := []struct {
		name string
		mod  module.Module
		mctx *module.Context
	}{
		{"prod-go", github.New(), nil},
		{"prod-py", githubpy.New(), nil},
		{"prod-ts", githubts.New(), nil},
		{"prod-mono github", githubmono.New(), &module.Context{Components: components}},
		{"prod-mono gitlab", gitlabmono.New(), gitlab},
		{"conformance github", conformance.New(), nil},
		{"conformance gitlab", conformance.New(), gitlab},
	}
	out := map[string][]byte{}
	for _, c := range cases {
		rs, err := c.mod.Resolve(context.Background(), c.mctx)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, r := range rs {
			if strings.HasSuffix(r.Path, ".yml") && (strings.Contains(r.Path, "workflows/") || strings.HasPrefix(r.Path, ".gitlab-ci")) {
				out[c.name+": "+r.Path] = r.Content
			}
		}
	}
	return out
}

// TestRenderedPins: changing the table changes every generated CI file that
// uses an entry, and nothing else in it (spec 0044 §3). Rendering with
// sentinel values and putting the real values back must give the same
// bytes, and every table entry must reach some file. With
// TestTemplatesHoldNoPins, that leaves the table as the only source.
func TestRenderedPins(t *testing.T) {
	real := pins.Current
	want := renderAll(t)
	if len(want) != 7 {
		t.Fatalf("rendered %d CI files, want 7: %v", len(want), keys(want))
	}

	sentinels := pins.Table{}
	sv := reflect.ValueOf(&sentinels).Elem()
	for i := range sv.NumField() {
		// Delimited, so no sentinel is a prefix of another (Task, TaskSHA256).
		sv.Field(i).SetString("<pin:" + sv.Type().Field(i).Name + ">")
	}
	pins.Current = sentinels
	t.Cleanup(func() { pins.Current = real })
	got := renderAll(t)

	used := map[string]bool{}
	realValues := fields(real)
	for file, content := range got {
		restored := string(content)
		for name, sentinel := range fields(sentinels) {
			if strings.Contains(restored, sentinel) {
				used[name] = true
			}
			restored = strings.ReplaceAll(restored, sentinel, realValues[name])
		}
		if restored != string(want[file]) {
			t.Errorf("%s: rendering with other pins changed more than the pinned values", file)
		}
	}
	for name := range realValues {
		if !used[name] {
			t.Errorf("table entry %s is used by no generated CI file", name)
		}
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
