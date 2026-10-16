package contract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// ErrNotWorkTree reports that the audit was pointed at a directory that is
// not inside a Git work tree. The audit reads tracked files only, so it
// cannot decide anything there (spec 0052, "What static checks read").
var ErrNotWorkTree = errors.New("not inside a Git work tree")

// Repo is the set of files a static check may read: the files git ls-files
// lists that exist in the working tree. An untracked file does not count,
// and neither does a tracked file deleted from the working tree.
type Repo struct {
	root  string
	files []string // slash separated, relative to root, sorted
}

// OpenRepo lists the tracked files of the Git work tree whose top level is
// root. It runs git and changes nothing.
func OpenRepo(ctx context.Context, root string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("cannot decide: git not found: %w", err)
	}

	inside, err := gitOutput(ctx, root, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return nil, fmt.Errorf("cannot decide: %s is %w; the audit reads tracked files only", root, ErrNotWorkTree)
	}
	prefix, err := gitOutput(ctx, root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, fmt.Errorf("cannot decide: %s: %w", root, err)
	}
	if p := strings.TrimSpace(string(prefix)); p != "" {
		return nil, fmt.Errorf("cannot decide: %s is not the top level of its Git work tree (it is %s inside it); "+
			"point --repo-root at the top level", root, strings.TrimSuffix(p, "/"))
	}

	tracked, err := gitOutput(ctx, root, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("cannot decide: git ls-files: %w", err)
	}
	deleted, err := gitOutput(ctx, root, "ls-files", "-z", "--deleted")
	if err != nil {
		return nil, fmt.Errorf("cannot decide: git ls-files --deleted: %w", err)
	}
	gone := map[string]bool{}
	for _, p := range splitNUL(deleted) {
		gone[p] = true
	}

	r := &Repo{root: root}
	for _, p := range splitNUL(tracked) {
		if !gone[p] {
			r.files = append(r.files, p)
		}
	}
	// A path appears once per stage during a merge conflict.
	slices.Sort(r.files)
	r.files = slices.Compact(r.files)
	return r, nil
}

// Root returns the directory the repository was opened at.
func (r *Repo) Root() string { return r.root }

// Files returns the tracked files, slash separated and sorted.
func (r *Repo) Files() []string { return slices.Clone(r.files) }

// HasFile reports whether path (slash separated, relative to the root) is
// a tracked file present in the working tree.
func (r *Repo) HasFile(path string) bool {
	_, found := slices.BinarySearch(r.files, path)
	return found
}

// HasDir reports whether dir (slash separated, relative to the root, "."
// for the root) holds at least one tracked file present in the working
// tree. A directory that holds none does not exist for the audit.
func (r *Repo) HasDir(dir string) bool {
	if dir == "." {
		return len(r.files) > 0
	}
	prefix := dir + "/"
	i, _ := slices.BinarySearch(r.files, prefix)
	return i < len(r.files) && strings.HasPrefix(r.files[i], prefix)
}

// ReadFile returns the working-tree content of a tracked file (slash
// separated, relative to the root). A file git ls-files does not list
// does not exist for the audit: the error wraps fs.ErrNotExist.
func (r *Repo) ReadFile(name string) ([]byte, error) {
	if !r.HasFile(name) {
		return nil, fmt.Errorf("%s: %w (not a tracked file)", name, fs.ErrNotExist)
	}
	return os.ReadFile(filepath.Join(r.root, filepath.FromSlash(name))) // #nosec G304 -- a tracked file of the audited repository
}

// FilesUnder returns the tracked files inside dir (slash separated, no
// trailing slash), sorted.
func (r *Repo) FilesUnder(dir string) []string {
	prefix := dir + "/"
	var out []string
	i, _ := slices.BinarySearch(r.files, prefix)
	for ; i < len(r.files) && strings.HasPrefix(r.files[i], prefix); i++ {
		out = append(out, r.files[i])
	}
	return out
}

func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- every caller passes literal git arguments; no path is passed as an argument
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out, nil
}

func splitNUL(b []byte) []string {
	var out []string
	for p := range bytes.SplitSeq(b, []byte{0}) {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out
}
