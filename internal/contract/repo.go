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
	// originHead is the branch refs/remotes/origin/HEAD points at, or ""
	// when the clone has none.
	originHead string
}

// originHead reads the branch refs/remotes/origin/HEAD points at. A clone
// without it (git symbolic-ref -q exits 1 and says nothing) gives "", for
// the fallback the CI rules assume (spec 0052, "Default branch"). Any
// other git failure is an error: the audit cannot tell the default
// branch, and assuming one would hide that.
func originHead(ctx context.Context, root string) (string, error) {
	out, err := gitOutput(ctx, root, "symbolic-ref", "-q", "refs/remotes/origin/HEAD")
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) && ge.exitCode() == 1 && ge.stderr == "" {
			return "", nil
		}
		return "", err
	}
	// A target outside refs/remotes/origin/ names no branch of origin.
	if b, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "refs/remotes/origin/"); ok {
		return b, nil
	}
	return "", nil
}

// OpenRepo lists the tracked files of the Git work tree whose top level is
// root. It runs git and changes nothing.
func OpenRepo(ctx context.Context, root string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("cannot decide: git not found: %w", err)
	}

	inside, err := gitOutput(ctx, root, "rev-parse", "--is-inside-work-tree")
	switch {
	case notARepository(err):
		return nil, fmt.Errorf("cannot decide: %s is %w; the audit reads tracked files only", root, ErrNotWorkTree)
	case err != nil:
		// Inside a work tree git can still refuse, as it does a repository
		// with dubious ownership or a broken configuration: that is a tool
		// error, with git's own message.
		return nil, fmt.Errorf("cannot decide: %w", err)
	case strings.TrimSpace(string(inside)) != "true":
		// Inside .git, or a bare repository.
		return nil, fmt.Errorf("cannot decide: %s is %w; the audit reads tracked files only", root, ErrNotWorkTree)
	}
	prefix, err := gitOutput(ctx, root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, fmt.Errorf("cannot decide: %w", err)
	}
	if p := strings.TrimSpace(string(prefix)); p != "" {
		return nil, fmt.Errorf("cannot decide: %s is not the top level of its Git work tree (it is %s inside it); "+
			"point --repo-root at the top level", root, strings.TrimSuffix(p, "/"))
	}

	tracked, err := gitOutput(ctx, root, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("cannot decide: %w", err)
	}
	deleted, err := gitOutput(ctx, root, "ls-files", "-z", "--deleted")
	if err != nil {
		return nil, fmt.Errorf("cannot decide: %w", err)
	}
	gone := map[string]bool{}
	for _, p := range splitNUL(deleted) {
		gone[p] = true
	}

	head, err := originHead(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("cannot decide: %w", err)
	}

	r := &Repo{root: root, originHead: head}
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

// NewRepo is a Repo whose tracked files the caller already knows, as a
// test knows the files of a fixture it built: files are slash separated,
// relative to root and present in the working tree. It runs no git, and
// the repository has no refs/remotes/origin/HEAD. The audit itself opens
// repositories with OpenRepo.
func NewRepo(root string, files []string) *Repo {
	r := &Repo{root: root, files: slices.Clone(files)}
	slices.Sort(r.files)
	r.files = slices.Compact(r.files)
	return r
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

// gitOutput runs git in dir and returns its standard output. Git runs
// with LC_ALL=C, so its messages are the untranslated ones notARepository
// reads. A failure is a *gitError.
func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- every caller passes literal git arguments; no path is passed as an argument
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(err, ctxErr) {
			err = fmt.Errorf("%w (%w)", ctxErr, err)
		}
		return nil, &gitError{args: args, dir: dir, stderr: strings.TrimSpace(stderr.String()), err: err}
	}
	return out, nil
}

// gitError is a git command that failed: what ran, where, how it ended,
// and what git said on stderr.
type gitError struct {
	args   []string
	dir    string
	stderr string
	// err is how the run ended: an *exec.ExitError, an error starting git,
	// or the context's error when the context ended the run.
	err error
}

func (e *gitError) Error() string {
	msg := fmt.Sprintf("git %s (in %s): %v", strings.Join(e.args, " "), e.dir, e.err)
	if e.stderr != "" {
		msg += ": " + e.stderr
	}
	return msg
}

func (e *gitError) Unwrap() error { return e.err }

// exitCode returns git's exit code, or -1 when git did not exit on its
// own.
func (e *gitError) exitCode() int {
	var exit *exec.ExitError
	if errors.As(e.err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

// notARepository reports whether err is git saying that its directory is
// not in a Git repository.
func notARepository(err error) bool {
	var ge *gitError
	return errors.As(err, &ge) && strings.Contains(ge.stderr, "not a git repository")
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
