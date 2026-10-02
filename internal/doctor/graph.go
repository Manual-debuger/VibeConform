package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

// Graph describes a code-intelligence graph a selected integration keeps
// in the repository, for the freshness and ignore checks. Doctor knows no
// integration by name: the CLI fills this in.
type Graph struct {
	// Name labels the results, such as "graphify".
	Name string
	// File is the graph, slash-separated relative to the repository root.
	File string
	// Commit is the top-level JSON key recording the commit the graph was
	// built from.
	Commit string
	// Rebuild is the command that rebuilds it, for the hints.
	Rebuild string
}

// GraphChecks reports whether g's graph exists and was built from HEAD,
// and whether Git ignores it, so derived output is not committed. A graph
// problem is only ever a warning: the graph is an optional aid, and
// nothing required stops without it (docs/specs/0035-graphify.md §3). It
// reads the graph as a stream, so a large graph is never held in memory.
func GraphChecks(ctx context.Context, env Env, repoRoot string, gitOK bool, g Graph) []Result {
	graph := Result{Name: g.Name + " graph"}
	ignored := Result{Name: g.Name + " ignore"}
	if !gitOK {
		graph.Status, graph.Detail = Unverified, "needs git"
		ignored.Status, ignored.Detail = Unverified, "needs git"
		return []Result{graph, ignored}
	}
	graph.Status, graph.Detail = graphFreshness(ctx, env, repoRoot, g)

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	_, stderr, err := env.Run(ctx, repoRoot, "git", "check-ignore", "-q", g.File)
	// Any error with an exit code, not only *exec.ExitError, so that a test
	// Env can say "exited 1".
	var exit interface{ ExitCode() int }
	switch {
	case err == nil:
		ignored.Status, ignored.Detail = Pass, g.File+" is ignored by git"
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		ignored.Status, ignored.Detail = Warn, g.File+" is not ignored by git (or is tracked); derived graph files could be committed"
	default:
		ignored.Status, ignored.Detail = Unverified, "git check-ignore failed: "+why(err, stderr)
	}
	return []Result{graph, ignored}
}

func graphFreshness(ctx context.Context, env Env, repoRoot string, g Graph) (Status, string) {
	f, err := env.Open(filepath.Join(repoRoot, filepath.FromSlash(g.File)))
	if errors.Is(err, fs.ErrNotExist) {
		return Warn, fmt.Sprintf("no graph (%s absent); run %s", g.File, g.Rebuild)
	}
	if err != nil {
		return Warn, fmt.Sprintf("%s unreadable (%v); rebuild with %s", g.File, err, g.Rebuild)
	}
	built, found, err := topLevelString(f, g.Commit)
	_ = f.Close() // read-only; a failed close loses nothing
	if err != nil {
		return Warn, fmt.Sprintf("%s unreadable (%v); rebuild with %s", g.File, err, g.Rebuild)
	}
	if !found || built == "" {
		return Unverified, fmt.Sprintf("freshness unknown: %s records no %s", g.File, g.Commit)
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, stderr, err := env.Run(ctx, repoRoot, "git", "rev-parse", "HEAD")
	if err != nil {
		return Unverified, "freshness unknown: git rev-parse HEAD failed: " + why(err, stderr)
	}
	head := firstLine(out)
	if !strings.EqualFold(built, head) {
		return Warn, fmt.Sprintf("stale: built at %s, HEAD %s; run %s", short(built), short(head), g.Rebuild)
	}
	detail := "current: built at HEAD " + short(head)
	if status, _, err := env.Run(ctx, repoRoot, "git", "status", "--porcelain", "--untracked-files=no"); err == nil && strings.TrimSpace(status) != "" {
		detail += "; uncommitted changes are not in the graph"
	}
	return Pass, detail
}

// topLevelString finds key among the top-level members of the JSON object
// r holds, skipping every other member's value token by token.
func topLevelString(r io.Reader, key string) (value string, found bool, err error) {
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil {
		return "", false, err
	} else if d, ok := t.(json.Delim); !ok || d != '{' {
		return "", false, errors.New("not a JSON object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return "", false, err
		}
		name, _ := t.(string)
		if name == key {
			var s string
			if err := dec.Decode(&s); err != nil {
				return "", false, fmt.Errorf("%s: %w", key, err)
			}
			return s, true, nil
		}
		if err := skipValue(dec); err != nil {
			return "", false, err
		}
	}
	return "", false, nil
}

// skipValue consumes one JSON value, however deeply nested.
func skipValue(dec *json.Decoder) error {
	depth := 0
	for {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
		if depth == 0 {
			return nil
		}
	}
}

// short abbreviates a commit hash for display.
func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}
