package guard

import (
	"os"
	"path/filepath"
	"strings"
)

// Repo is the repository a tool call acts in (spec 0053, "Repository
// root"). It is found by reading files, never by running git, so the
// guard works with only the operating system's PATH (A-03).
type Repo struct {
	// Root is the directory holding .git, or the start directory when no
	// .git was found above it.
	Root string
	// GitDir is the repository's Git directory: Root/.git, or where a
	// .git file's "gitdir:" line points. It is "" when none was found or
	// the .git file cannot be read.
	GitDir string
	// CommonDir is the common Git directory, shared by every linked
	// worktree: GitDir, or where GitDir's commondir file points. The
	// unlock marker and vibe-guard.log live in it. It is "" when it is not
	// known; an unknown common directory never holds a valid marker.
	CommonDir string
}

// FindRepo walks up from start to the directory that holds .git.
func FindRepo(start string) Repo {
	if abs, err := filepath.Abs(start); err == nil {
		start = abs
	}
	for dir := start; ; {
		dotGit := filepath.Join(dir, ".git")
		if fi, err := os.Stat(dotGit); err == nil {
			r := Repo{Root: dir}
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
			return Repo{Root: start}
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
	return resolve(root, strings.TrimSpace(p))
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
	return resolve(gitDir, p)
}

func resolve(base, p string) string {
	p = filepath.FromSlash(p)
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}
