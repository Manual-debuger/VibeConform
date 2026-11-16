package contract

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// The V-07 sample is a PEM private-key block in the shape gitleaks's
// private-key rule reports: a BEGIN line, at least 64 characters of
// base64 body, and the matching END line (spec 0061, "V-07, secret").
// It is the same in every language, in a file with a neutral name, and
// fixed. The patterns below are not a literal header: no file in this
// repository holds one (AC 6).
func TestSecretSample(t *testing.T) {
	t.Parallel()
	env, comp := goTestRepo(t)
	d, err := defectFor(env, comp, defectSecret)
	if err != nil {
		t.Fatal(err)
	}
	if d.Path != "foo/vibe_probe_secret.txt" {
		t.Errorf("path = %q, want foo/vibe_probe_secret.txt", d.Path)
	}
	shape := regexp.MustCompile(`(?m)^-{5}BEGIN PRIVATE KEY-{5}\n((?:[A-Za-z0-9+/=]+\n)+)-{5}END PRIVATE KEY-{5}\n`)
	m := shape.FindSubmatch(d.Content)
	if m == nil {
		t.Fatalf("the sample is not a PEM private-key block:\n%s", d.Content)
	}
	body := strings.ReplaceAll(string(m[1]), "\n", "")
	if len(body) < 64 {
		t.Errorf("the body has %d base64 characters, want at least 64", len(body))
	}
	if _, err := base64.StdEncoding.DecodeString(body); err != nil {
		t.Errorf("the body is not base64: %v", err)
	}
	for _, lang := range languageRegistry {
		if s, _ := lang.sample(defectSecret); s != secretSample {
			t.Errorf("the %s secret sample differs from the shared one: %+v", lang.id, s)
		}
	}
	again, err := defectFor(env, comp, defectSecret)
	if err != nil || !bytes.Equal(again.Content, d.Content) {
		t.Errorf("the sample is not fixed: %q, then %q (%v)", d.Content, again.Content, err)
	}
}

// No file in this repository holds a literal PEM private-key header
// (spec 0061, AC 6): vibe assembles V-07's sample at run time, so a
// secret scanner finds nothing in vibe's own source. The pattern counts
// the dashes, so that this file holds none either.
func TestNoLiteralPrivateKeyHeader(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	repo, err := OpenRepo(context.Background(), root)
	if err != nil {
		t.Skipf("cannot list this checkout's files: %v", err)
	}
	header := regexp.MustCompile(`-{5}BEGIN[ A-Z0-9_-]*PRIVATE KEY`)
	for _, f := range repo.Files() {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))) // #nosec G304 -- this repository's tracked files
		if err != nil {
			continue
		}
		if header.Match(data) {
			t.Errorf("%s holds a literal PEM private-key header", f)
		}
	}
}
