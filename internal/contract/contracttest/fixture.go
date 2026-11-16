// Package contracttest is the test seam of the contract audit (ADR 0028,
// spec 0065). It builds fixture repositories from the trees under
// internal/contract/testdata, opens them without git when it can, and
// runs contract.Load and contract.Audit on them, checking every report
// it returns. internal/contract's tests and internal/cli's import it, so
// the fixture builder exists once. Only test code imports it: a depguard
// rule in .golangci.yml keeps it out of every other package.
package contracttest

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// The fixture trees. Each is an absolute path, found from this file's
// location, so that a test in any package can use it.
var (
	// Baseline is the tree every rule fixture starts from: a repository
	// that passes every static part.
	Baseline = tree("baseline")
	// ProbeGo is the Go probe fixture (spec 0053): its entrypoints and
	// hooks run go run ./ci <step>.
	ProbeGo = tree("probe-go")
	// ProbePython is the Python probe fixture; it needs uv.
	ProbePython = tree("probe-python")
	// ProbeTypeScript is the TypeScript probe fixture; it needs pnpm.
	ProbeTypeScript = tree("probe-typescript")
)

// tree is the absolute path of a tree under internal/contract/testdata.
func tree(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		panic("contracttest: cannot find its own source file, so it cannot find internal/contract/testdata")
	}
	return filepath.Join(filepath.Dir(file), "..", "testdata", name)
}

// fixtureDotClaude is the name testdata uses for a fixture's .claude
// directory. The agent guard refuses edits of any path ending in
// .claude/hooks/policy.json, including fixtures, so testdata keeps them
// under dot-claude/ and the copy renames it.
const fixtureDotClaude = "dot-claude"

// fixturePath maps a path in testdata (OS separators) to the path in the
// fixture: every dot-claude component becomes .claude.
func fixturePath(rel string) string {
	parts := strings.Split(rel, string(filepath.Separator))
	for i, p := range parts {
		if p == fixtureDotClaude {
			parts[i] = ".claude"
		}
	}
	return filepath.Join(parts...)
}

// Edit changes a fixture before or after git add.
type Edit struct {
	path      string
	content   string
	remove    bool
	untracked bool
}

// WithFile writes path (slash separated) with content before git add, so
// the file is tracked. A path under .claude is written as is.
func WithFile(path, content string) Edit {
	return Edit{path: path, content: content}
}

// WithoutFile removes path (slash separated) from the fixture before git
// add.
func WithoutFile(path string) Edit {
	return Edit{path: path, remove: true}
}

// WithUntracked writes path after git add, so the file exists in the
// working tree but is not tracked. It makes the fixture a Git
// repository.
func WithUntracked(path, content string) Edit {
	return Edit{path: path, content: content, untracked: true}
}

// Under returns edits with each path moved under dir (slash separated),
// such as a component's directory.
func Under(dir string, edits ...Edit) []Edit {
	out := make([]Edit, len(edits))
	for i, e := range edits {
		e.path = dir + "/" + e.path
		out[i] = e
	}
	return out
}

// Fixture copies the tree src into a new temporary directory, applies
// edits and returns its root. src is one of the trees above or any
// directory, an empty one too. Starting git costs a table case most of
// its time (spec 0056), so a fixture whose files are all tracked is a
// plain directory whose file list OpenRepo hands to the audit: a listed
// fixture. A fixture with an untracked file or a .gitignore is a Git
// repository, because only git decides what it tracks.
func Fixture(t testing.TB, src string, edits ...Edit) string {
	t.Helper()
	return build(t, src, false, edits)
}

// GitFixture is Fixture, but the fixture is always a Git repository with
// every file but the untracked ones added: for the tests that read git
// itself (the default branch, git failures) and for the probes, whose
// probe copy is made with git.
func GitFixture(t testing.TB, src string, edits ...Edit) string {
	t.Helper()
	return build(t, src, true, edits)
}

// listed maps the root of each listed fixture to its files, slash
// separated.
var listed sync.Map

// OpenRepo is a contract.RepoOpener for fixtures: a listed fixture gets
// its file list, any other directory contract.OpenRepo.
func OpenRepo(ctx context.Context, root string) (*contract.Repo, error) {
	if files, ok := listed.Load(filepath.Clean(root)); ok {
		return contract.NewRepo(root, files.([]string)), nil
	}
	return contract.OpenRepo(ctx, root)
}

func build(t testing.TB, src string, git bool, edits []Edit) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, fixturePath(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o750)
		}
		data, err := os.ReadFile(p) // #nosec G304 G122 -- a test's own fixture tree
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o600) // #nosec G703 -- a path under the test's temporary directory
	})
	if err != nil {
		t.Fatalf("copying the fixture tree %s: %v", src, err)
	}

	for _, e := range edits {
		if !e.untracked {
			applyEdit(t, dir, e)
		} else {
			git = true
		}
	}
	files := fixtureFiles(t, dir)
	if !git && !slices.ContainsFunc(files, func(f string) bool { return path.Base(f) == ".gitignore" }) {
		listed.Store(filepath.Clean(dir), files)
		t.Cleanup(func() { listed.Delete(filepath.Clean(dir)) })
		return dir
	}
	Git(t, dir, "init", "-q")
	Git(t, dir, "-c", "core.autocrlf=false", "add", "-A")
	for _, e := range edits {
		if e.untracked {
			applyEdit(t, dir, e)
		}
	}
	return dir
}

// fixtureFiles lists the files under dir, slash separated and relative to
// it: what git add -A would track in a tree without a .gitignore.
func fixtureFiles(t testing.TB, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("listing the fixture: %v", err)
	}
	return files
}

func applyEdit(t testing.TB, dir string, e Edit) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(e.path))
	if e.remove {
		if err := os.Remove(p); err != nil {
			t.Fatalf("removing %s: %v", e.path, err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(e.content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Git runs git with args in dir, such as a Git fixture, and fails the
// test when git fails.
func Git(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) // #nosec G204 -- a test's own arguments
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
