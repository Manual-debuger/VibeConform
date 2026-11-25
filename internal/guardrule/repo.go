package guardrule

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"
)

// Repo is the repository a tool call acts in (spec 0053, "Repository
// root"; spec 0068). It is found by reading files, never by running
// git, so the guard works with only the operating system's PATH (A-03).
// vibe-guard and vibe unlock both find it with FindRepo, so the unlock
// marker is written where the guard reads it (spec 0069).
type Repo struct {
	// Root is the project root, the directory the agent session runs in.
	// The policy, exception globs, relative paths and the guardrail
	// settings are read against it.
	Root string
	// GitTop is the directory that holds .git, found by walking up from
	// Root. It is Root unless the project is below the Git top, and ""
	// when no .git was found.
	GitTop string
	// GitDir is the repository's Git directory: GitTop/.git, or where a
	// .git file's "gitdir:" line points. It is "" when none was found or
	// the .git file cannot be read.
	GitDir string
	// CommonDir is the common Git directory, shared by every linked
	// worktree: GitDir, or where GitDir's commondir file points. The
	// unlock marker and vibe-guard.log live in it. It is "" when it is not
	// known; an unknown common directory never holds a valid marker.
	CommonDir string
}

// FindRepo walks up from start to the Git top, the directory that
// holds .git, and finds the Git directories from it. The project root
// is root, or, when root is "", the Git top. Without .git above start,
// the project root is root, else start, and there is no Git top.
// GIT_DIR and GIT_COMMON_DIR are not read.
func FindRepo(start, root string) Repo {
	if abs, err := filepath.Abs(start); err == nil {
		start = abs
	}
	for dir := start; ; {
		dotGit := filepath.Join(dir, ".git")
		if fi, err := os.Stat(dotGit); err == nil {
			r := Repo{Root: cmp.Or(root, dir), GitTop: dir}
			if fi.IsDir() {
				r.GitDir = dotGit
			} else {
				r.GitDir = readGitFile(dir, dotGit)
			}
			if r.GitDir != "" {
				r.CommonDir = commonDir(r.GitDir)
			}
			return r
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Repo{Root: cmp.Or(root, start)}
		}
		dir = parent
	}
}

// readGitFile reads a .git file ("gitdir: <path>"), as a linked worktree
// or a submodule has.
func readGitFile(root, file string) string {
	data, err := os.ReadFile(file) // #nosec G304 -- the .git file of the repository being guarded
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	p, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return ""
	}
	return gitPath(root, strings.TrimSpace(p))
}

// commonDir follows gitDir's commondir file, which a linked worktree's
// Git directory has.
func commonDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir")) // #nosec G304 -- inside the repository's own Git directory
	if err != nil {
		if os.IsNotExist(err) {
			return gitDir
		}
		return ""
	}
	p := strings.TrimSpace(string(data))
	if p == "" {
		return ""
	}
	return gitPath(gitDir, p)
}

// gitPath is a path read from a .git or commondir file, made absolute
// against base, the directory the file's paths are relative to.
func gitPath(base, p string) string {
	p = filepath.FromSlash(p)
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}
