// Package setupskill holds the setup skill, vibeconform-setup (spec 0054,
// "The setup skill"): a skill directory embedded in vibe that takes a
// repository to a passing contract audit, and the code that writes it
// into a repository (vibe init --skill).
//
// The skill has two placeholders, which Render replaces in every file:
// {{VIBE_VERSION}} (VersionPlaceholder), the vibe version that the
// vibe-guard install hint names, and {{VIBE_RELEASE_PAGE}}
// (ReleasePagePlaceholder), that version's release page. Render
// changes nothing else.
//
// Among this repository's packages it imports only internal/guard, for
// the version rule, the release page and the reference hook commands.
package setupskill

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// Name is the skill's name and the name of its directory.
const Name = "vibeconform-setup"

// The skill's placeholders. Render replaces each of them, and nothing
// else, in every file of the skill (spec 0073):
//
//   - VersionPlaceholder is the vibe version that the install hint
//     names, from internal/guard's version rule (guard.InstallVersion).
//   - ReleasePagePlaceholder is the page that publishes that version's
//     release archives (guard.ReleasePage): the tag's page, or the
//     releases page for latest.
const (
	VersionPlaceholder     = "{{VIBE_VERSION}}"
	ReleasePagePlaceholder = "{{VIBE_RELEASE_PAGE}}"
)

// Dirs returns the directories, relative to the repository root and in
// slash form, that Install writes: Claude Code's skill path and Codex's.
// Each call returns a new slice.
func Dirs() []string {
	return []string{".claude/skills/" + Name, ".agents/skills/" + Name}
}

//go:embed all:skill
var embedded embed.FS

// skill is the embedded skill directory as a file system of its own.
var skill = func() fs.FS {
	sub, err := fs.Sub(embedded, "skill")
	if err != nil {
		panic(err) // the directory is embedded, so this cannot fail
	}
	return sub
}()

// FS is the embedded skill as it is stored, with the placeholders in
// place: SKILL.md at its root. Every call returns the same file system.
func FS() fs.FS {
	return skill
}

// Render returns every file of the skill, by slash path relative to the
// skill directory, with the placeholders replaced for a vibe of version
// vibeVersion: the version the install hint names
// (guard.InstallVersion) and its release page (guard.ReleasePage).
func Render(vibeVersion string) (map[string][]byte, error) {
	version := guard.InstallVersion(vibeVersion)
	placeholders := strings.NewReplacer(
		VersionPlaceholder, version,
		ReleasePagePlaceholder, guard.ReleasePage(version),
	)
	out := map[string][]byte{}
	err := fs.WalkDir(skill, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(skill, p)
		if err != nil {
			return err
		}
		out[p] = []byte(placeholders.Replace(string(data)))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the embedded skill: %w", err)
	}
	return out, nil
}

// Result is what Install did: the paths it wrote and removed, relative
// to the repository root, in slash form and sorted.
type Result struct {
	Written []string
	Removed []string
}

// Install writes the skill into both of Dirs() under repoRoot (spec 0054,
// "vibe init --skill"). Every file is replaced with the rendered
// version, files the skill does not have are removed, and directories
// that removal empties are removed too. Nothing outside the two
// directories is written.
func Install(repoRoot, vibeVersion string) (Result, error) {
	files, err := Render(vibeVersion)
	if err != nil {
		return Result{}, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var res Result
	for _, dir := range Dirs() {
		if err := install(filepath.Join(repoRoot, filepath.FromSlash(dir)), dir, files, names, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// install writes files into the directory at path, which Result names
// as dir, after pruning it.
func install(path, dir string, files map[string][]byte, names []string, res *Result) error {
	removed, err := prune(path, files)
	if err != nil {
		return fmt.Errorf("prune %s: %w", dir, err)
	}
	for _, r := range removed {
		res.Removed = append(res.Removed, pathpkg.Join(dir, r))
	}
	for _, name := range names {
		target := filepath.Join(path, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("write %s: %w", pathpkg.Join(dir, name), err)
		}
		if err := os.WriteFile(target, files[name], 0o600); err != nil {
			return fmt.Errorf("write %s: %w", pathpkg.Join(dir, name), err)
		}
		res.Written = append(res.Written, pathpkg.Join(dir, name))
	}
	return nil
}

// prune removes every file under root that keep does not name, and every
// directory left empty, and returns the removed files' slash paths
// relative to root. A missing root has nothing to prune. The walk only
// collects; the removals follow it.
func prune(root string, keep map[string][]byte) ([]string, error) { //nolint:gocognit // baseline: pre-existing complexity
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var stale, dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir():
			if rel != "." {
				dirs = append(dirs, rel)
			}
		case !d.Type().IsRegular():
			stale = append(stale, rel)
		default:
			if _, ok := keep[rel]; !ok {
				stale = append(stale, rel)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, rel := range stale {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return nil, err
		}
	}
	// Deepest first, so that a directory is empty once its children are
	// gone. A directory that still holds a file is kept.
	slices.Reverse(dirs)
	for _, rel := range dirs {
		d := filepath.Join(root, filepath.FromSlash(rel))
		if entries, err := os.ReadDir(d); err == nil && len(entries) == 0 {
			if err := os.Remove(d); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(stale)
	return stale, nil
}
