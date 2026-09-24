package module

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// verifyFastSteps is spec 0023's section 2 table: the part of verify that
// is cheap and incremental, in verify's own order. The Stop hook and the
// async check both run verify:fast, so a slow or network-bound step added
// here slows every agent turn.
var verifyFastSteps = map[string][]string{
	"repotooling/templates/Taskfile.yml":   {"fmt:check", "typecheck", "lint", "test"},
	"tsrepotooling/templates/Taskfile.yml": {"fmt:check", "lint", "typecheck", "test"},
	"pyrepotooling/templates/Taskfile.yml": {"fmt:check", "lint", "typecheck", "test"},
}

// describedTaskfile is taskfileDoc plus each task's desc, which the
// verify-independence tests don't need.
type describedTaskfile struct {
	Tasks map[string]struct {
		Desc string    `yaml:"desc"`
		Cmds []taskCmd `yaml:"cmds"`
	} `yaml:"tasks"`
}

func readTaskfile(t *testing.T, path string) (taskfileDoc, describedTaskfile) {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var doc taskfileDoc
	var described describedTaskfile
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if err := yaml.Unmarshal(data, &described); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return doc, described
}

// TestVerifyFastIsTheIncrementalSubset pins spec 0023's verify:fast: a
// public task (people run it, so it has a desc) that runs exactly the
// listed subtasks and nothing else, and whose steps verify also runs, so
// passing verify:fast never means skipping something verify:fast claims.
func TestVerifyFastIsTheIncrementalSubset(t *testing.T) {
	for _, path := range repoToolingTaskfiles {
		t.Run(path, func(t *testing.T) {
			_, tf := readTaskfile(t, path)

			fast, ok := tf.Tasks["verify:fast"]
			if !ok {
				t.Fatal("no verify:fast task; the Stop hook and the async check run it")
			}
			if fast.Desc == "" {
				t.Error("verify:fast has no desc; people run it too, so it belongs in task --list")
			}

			var got []string
			for _, c := range fast.Cmds {
				if c.dep == "" {
					t.Errorf("verify:fast runs shell command %q; it should only call verify's own subtasks", c.shell)
					continue
				}
				got = append(got, c.dep)
			}
			if want := verifyFastSteps[path]; !slices.Equal(got, want) {
				t.Errorf("verify:fast runs %v, want %v", got, want)
			}

			var verifySteps []string
			for _, c := range tf.Tasks["verify"].Cmds {
				verifySteps = append(verifySteps, c.dep)
			}
			for _, step := range got {
				if !slices.Contains(verifySteps, step) {
					t.Errorf("verify:fast runs %s, which verify does not", step)
				}
			}
		})
	}
}

// uncacheableTestFlags are the go test flags that disable test caching
// (go help test: only a fixed set of flags is cacheable, and -count is the
// usual way to force a rerun). Spec 0023 relies on the test cache to rerun
// only affected packages, so none of these may appear in what verify:fast
// runs.
var uncacheableTestFlags = []string{"-count", "-coverprofile", "-cpuprofile", "-memprofile", "-blockprofile", "-mutexprofile", "-trace", "-outputdir", "-benchmem", "-bench"}

// TestGoTestStaysCacheable pins that verify:fast's go test keeps the test
// cache: package arguments (a bare go test is never cached) and no flag
// outside the cacheable set.
func TestGoTestStaysCacheable(t *testing.T) {
	doc, _ := readTaskfile(t, "repotooling/templates/Taskfile.yml")

	found := false
	for _, cmd := range doc.shellClosure(t, "verify:fast") {
		fields := strings.Fields(cmd)
		if len(fields) < 2 || fields[0] != "go" || fields[1] != "test" {
			continue
		}
		found = true
		if !slices.ContainsFunc(fields[2:], func(f string) bool { return strings.HasPrefix(f, "./") || f == "." }) {
			t.Errorf("%q passes no package argument; go test without one is never cached", cmd)
		}
		for _, f := range fields[2:] {
			name, _, _ := strings.Cut(f, "=")
			if slices.Contains(uncacheableTestFlags, name) {
				t.Errorf("%q passes %s, which disables the test cache", cmd, name)
			}
		}
	}
	if !found {
		t.Fatal("verify:fast runs no go test; the affected-tests guarantee depends on it")
	}
}
