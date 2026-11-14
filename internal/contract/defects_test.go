package contract

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// goTestRepo is a Go component at the root whose only test file, in
// foo/, is an external test package.
func goTestRepo(t *testing.T) (*Env, Component) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":          "module example.com/m\n",
		"foo/foo.go":      "package foo\n",
		"foo/foo_test.go": "package foo_test\n",
	}
	var list []string
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		list = append(list, name)
	}
	comp := Component{Path: ".", Language: LanguageGo}
	return newEnv(context.Background(), &Config{Components: []Component{comp}}, NewRepo(root, list)), comp
}

// A Go sample takes the package clause of the test file beside it. A
// non-test sample drops the external test package's _test suffix, since
// only a _test.go file may declare foo_test (spec 0061, "The Go V-06
// sample is a non-test file").
func TestGoSamplePackageClause(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, sample, want string
	}{
		{"non-test sample", "vibe_probe_sample.go", "package foo\n"},
		{"test sample", "vibe_probe_sample_test.go", "package foo_test\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env, comp := goTestRepo(t)
			d, err := plantSample(env, comp, defectSample{Name: tc.sample, Content: "package {{package}}\n"})
			if err != nil {
				t.Fatal(err)
			}
			if d.Path != "foo/"+tc.sample {
				t.Errorf("path = %q, want foo/%s", d.Path, tc.sample)
			}
			if string(d.Content) != tc.want {
				t.Errorf("content = %q, want %q", d.Content, tc.want)
			}
		})
	}
}
