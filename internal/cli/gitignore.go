package cli

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// checkIgnored is a test seam, like installGitHooks: the CLI suite plans
// repositories in t.TempDir(), which is not a git work tree.
var checkIgnored = gitCheckIgnored

// errNoGit reports that the ignored-path check could not run: no git on
// PATH, or repoRoot is not inside a work tree. Plans carry it as a
// warning, never as a failure (docs/specs/0026-optional-integrations.md).
var errNoGit = errors.New("not checked whether git ignores managed files")

// gitCheckIgnored returns the paths, of those given, that Git ignores and
// does not track. A managed file like that is written by sync and then
// never committed, so every fresh checkout — CI's included — is missing
// it. One git call covers every path.
func gitCheckIgnored(ctx context.Context, repoRoot string, paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errNoGit
	}

	probe := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	probe.Dir = repoRoot
	if out, err := probe.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil, errNoGit
	}

	// Literal command and arguments; the paths travel on stdin, NUL
	// separated, so no path is ever parsed as a flag. Tracked files are
	// not reported without --no-index, which is what we want: a committed
	// file is in every checkout whatever .gitignore says.
	c := exec.CommandContext(ctx, "git", "check-ignore", "--stdin", "-z")
	c.Dir = repoRoot
	c.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := c.Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return nil, nil // exit 1: none of the paths is ignored
	default:
		return nil, err
	}

	ignored := map[string]bool{}
	for p := range bytes.SplitSeq(out, []byte{0}) {
		if len(p) > 0 {
			ignored[string(p)] = true
		}
	}
	return ignored, nil
}
