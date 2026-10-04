package doctor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// AgentHook is one selected agent integration's hooks, as the CLI knows
// them from the resolved plan.
type AgentHook struct {
	// Name is the integration, e.g. "claude".
	Name string
	// Config is the repository-relative, slash-separated file that
	// registers the hooks. Empty when Suspended is set.
	Config string
	// Suspended, when set, says why this agent's hooks are not generated,
	// and the check reports it instead of probing anything.
	Suspended string
	// Binaries are what every hook command starts: task, then the guard's
	// runtime.
	Binaries []string
}

// AgentHooks checks that each selected agent's hooks can fire: the config
// that registers them is on disk, and every binary they start is on PATH.
// Whether the config's content is right is vibe audit's question.
func AgentHooks(env Env, repoRoot string, hooks []AgentHook) []Result {
	results := make([]Result, 0, len(hooks))
	for _, h := range hooks {
		res := Result{Name: h.Name + " hooks"}
		switch {
		case h.Suspended != "":
			res.Status, res.Detail = Pass, h.Suspended
		case !env.Exists(filepath.Join(repoRoot, filepath.FromSlash(h.Config))):
			res.Status, res.Detail = Fail, h.Config+" is missing; run vibe sync"
		default:
			var missing, found []string
			for _, bin := range h.Binaries {
				path, err := env.LookPath(bin)
				if err != nil {
					missing = append(missing, bin)
					continue
				}
				found = append(found, bin+" ("+path+")")
			}
			if len(missing) > 0 {
				res.Status = Fail
				res.Detail = fmt.Sprintf("%s present, but %s not on PATH: the hooks, the guard included, cannot run", h.Config, strings.Join(missing, ", "))
			} else {
				res.Status = Pass
				res.Detail = fmt.Sprintf("%s present; on PATH: %s", h.Config, strings.Join(found, ", "))
			}
		}
		results = append(results, res)
	}
	return results
}

// LineEndings reports how git will check out a managed file: the text and
// eol attributes it resolves for path, and core.autocrlf. A report only;
// enforcing line endings belongs to the text policy, not to doctor.
func LineEndings(ctx context.Context, env Env, repoRoot, path string) Result {
	res := Result{Name: "line endings"}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	autocrlf, eol, failed := attributes(ctx, env, repoRoot, path)
	if failed != "" {
		res.Status, res.Detail = Unverified, failed
		return res
	}

	res.Detail = fmt.Sprintf("core.autocrlf=%s; %s has eol=%s", autocrlf, path, eol)
	switch {
	case eol == "lf":
		res.Status = Pass
	case eol == "crlf":
		res.Status = Warn
		res.Detail += ": managed files check out as CRLF"
	case autocrlf == "true":
		res.Status = Warn
		res.Detail = fmt.Sprintf("core.autocrlf=true and %s has no eol attribute: managed files check out as CRLF", path)
	default:
		res.Status = Pass
		res.Detail += " (not pinned)"
	}
	return res
}

// LineEndingPolicy reports the health of a selected policy.line_endings: lf
// (spec 0029 §7). The effective eol of a managed file must be lf, whatever
// overrode the policy if it is not; and files committed with CRLF before
// the policy still need renormalizing. A report only: audit owns the
// section itself.
func LineEndingPolicy(ctx context.Context, env Env, repoRoot, path string) Result {
	res := Result{Name: "line endings"}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	autocrlf, eol, failed := attributes(ctx, env, repoRoot, path)
	if failed != "" {
		res.Status, res.Detail = Unverified, failed
		return res
	}
	if eol != "lf" {
		res.Status = Fail
		res.Detail = fmt.Sprintf("policy.line_endings is lf, but %s has eol=%s: a rule in a nested .gitattributes, .git/info/attributes, or core.attributesFile overrides it", path, eol)
		return res
	}

	out, stderr, err := env.Run(ctx, repoRoot, "git", "ls-files", "--eol")
	if err != nil {
		res.Status, res.Detail = Warn, "policy line_endings: lf; could not list how tracked files are stored: "+why(err, stderr)
		return res
	}
	crlf := 0
	for l := range strings.SplitSeq(out, "\n") {
		// "i/crlf  w/crlf  attr/text=auto eol=lf <TAB>path": the index side
		// is what the next checkout materializes from.
		if strings.HasPrefix(l, "i/crlf") {
			crlf++
		}
	}
	if crlf > 0 {
		res.Status = Warn
		res.Detail = fmt.Sprintf("policy line_endings: lf, but %d tracked %s stored with CRLF; run git add --renormalize . and commit", crlf, plural(crlf, "file is", "files are"))
		return res
	}
	res.Status = Pass
	res.Detail = fmt.Sprintf("policy line_endings: lf; core.autocrlf=%s; %s has eol=lf", autocrlf, path)
	return res
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// attributes returns core.autocrlf and the eol attribute git resolves for
// path, or why it could not tell.
func attributes(ctx context.Context, env Env, repoRoot, path string) (autocrlf, eol, failed string) {
	if path == "" {
		return "", "", "the standard generates no whole file to check"
	}
	autocrlf = "unset"
	if out, _, err := env.Run(ctx, repoRoot, "git", "config", "--get", "core.autocrlf"); err == nil {
		if v := firstLine(out); v != "" {
			autocrlf = v
		}
	}
	out, stderr, err := env.Run(ctx, repoRoot, "git", "check-attr", "eol", "--", path)
	if err != nil {
		return "", "", "git check-attr failed: " + why(err, stderr)
	}
	// git check-attr prints "<path>: eol: <value>".
	line := firstLine(out)
	i := strings.LastIndex(line, ": ")
	if i < 0 {
		return "", "", fmt.Sprintf("git check-attr printed %q, not an attribute", line)
	}
	return autocrlf, line[i+2:], ""
}

// RuntimeMarker is a file whose existence identifies where the process
// runs. hook:context tests the same paths, with the shell's own test -e;
// a test holds the two lists together.
type RuntimeMarker struct {
	Path    string
	Runtime string
}

// RuntimeMarkers are checked in order; the first that exists wins. Only
// positive markers: their absence is reported as native, which claims no
// more than "no marker found".
var RuntimeMarkers = []RuntimeMarker{
	{Path: "/proc/sys/fs/binfmt_misc/WSLInterop", Runtime: "wsl"},
	{Path: "/.dockerenv", Runtime: "container"},
	{Path: "/run/.containerenv", Runtime: "container"},
}

// Runtime reports the platform and whether a WSL or container marker
// exists. The markers are Linux paths, so they are only probed on Linux.
func Runtime(env Env) Result {
	rt := "native"
	if env.GOOS == "linux" {
		for _, m := range RuntimeMarkers {
			if env.Exists(m.Path) {
				rt = m.Runtime
				break
			}
		}
	}
	return Result{Status: Pass, Name: "runtime", Detail: fmt.Sprintf("%s/%s, %s", env.GOOS, env.GOARCH, rt)}
}

// Worktree reports whether repoRoot is a linked worktree. A linked one is
// UNVERIFIED rather than PASS: it shares its git directory, and so the Git
// hooks lefthook installs, with every other worktree, and no probe for
// that kind of collision is reliable yet.
func Worktree(ctx context.Context, env Env, repoRoot string) Result {
	res := Result{Name: "worktree"}
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	gitDir, stderr, err := env.Run(ctx, repoRoot, "git", "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		res.Status, res.Detail = Unverified, "git rev-parse failed: "+why(err, stderr)
		return res
	}
	common, stderr, err := env.Run(ctx, repoRoot, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		res.Status, res.Detail = Unverified, "git rev-parse failed: "+why(err, stderr)
		return res
	}
	if firstLine(gitDir) == firstLine(common) {
		res.Status, res.Detail = Pass, "main checkout, not a linked worktree"
		return res
	}
	res.Status = Unverified
	res.Detail = fmt.Sprintf("linked worktree (main checkout at %s); collisions with other worktrees (shared Git hooks, tool caches) not checked",
		strings.TrimSuffix(firstLine(common), "/.git"))
	return res
}
