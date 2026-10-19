// Package setupskill holds the setup skill, vibeconform-setup (spec 0054,
// "The setup skill"): a skill directory embedded in vibe that takes a
// repository to a passing contract audit, and the code that writes it
// into a repository (vibe init --skill).
//
// It stands beside the template-sync system and imports none of its
// packages, so that #112 can remove them. Among this repository's
// packages it imports only internal/guard, for the version rule.
package setupskill

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// Name is the skill's name and the name of its directory.
const Name = "vibeconform-setup"

// VersionPlaceholder is the only placeholder in the skill: the vibe
// version, replaced by Render through internal/guard's version rule.
const VersionPlaceholder = "{{VIBE_VERSION}}"

// Dirs are the directories, relative to the repository root and in slash
// form, that Install writes: Claude Code's skill path and Codex's.
var Dirs = []string{".claude/skills/" + Name, ".agents/skills/" + Name}

//go:embed all:skill
var embedded embed.FS

// FS is the embedded skill as it is stored, with the placeholder in
// place: SKILL.md at its root.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "skill")
	if err != nil {
		panic(err) // the directory is embedded, so this cannot fail
	}
	return sub
}

// Render returns every file of the skill, by slash path relative to the
// skill directory, with the placeholder replaced by the version the
// install hint names for a vibe of version vibeVersion
// (guard.InstallVersion).
func Render(vibeVersion string) (map[string][]byte, error) {
	version := []byte(guard.InstallVersion(vibeVersion))
	// latest has no release of its own: a link to its tag becomes the
	// releases page, as in guard.ReferencePreToolUse.
	latestTag := []byte("/releases/tag/latest")
	out := map[string][]byte{}
	err := fs.WalkDir(FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(FS(), p)
		if err != nil {
			return err
		}
		data = bytes.ReplaceAll(data, []byte(VersionPlaceholder), version)
		out[p] = bytes.ReplaceAll(data, latestTag, []byte("/releases"))
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

// Install writes the skill into both of Dirs under repoRoot (spec 0054,
// "vibe init --skill"). Every file is replaced with the rendered
// version, files the skill does not have are removed, and directories
// that removal empties are removed too. Nothing outside the two
// directories is read or written: each is opened as an os.Root, so a
// symbolic link inside cannot lead a write or a removal out of it.
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
	for _, dir := range Dirs {
		if err := install(filepath.Join(repoRoot, filepath.FromSlash(dir)), dir, files, names, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// install writes files into the directory at path, which Result names
// as dir, after pruning it.
func install(path, dir string, files map[string][]byte, names []string, res *Result) error {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("write %s: %w", dir, err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return fmt.Errorf("write %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()

	removed, err := prune(root, files)
	if err != nil {
		return fmt.Errorf("prune %s: %w", dir, err)
	}
	for _, r := range removed {
		res.Removed = append(res.Removed, pathpkg.Join(dir, r))
	}
	for _, name := range names {
		if err := root.MkdirAll(pathpkg.Dir(name), 0o750); err != nil {
			return fmt.Errorf("write %s: %w", pathpkg.Join(dir, name), err)
		}
		// A skill is read by every agent and person working in the
		// repository, like any other committed file.
		if err := root.WriteFile(name, files[name], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", pathpkg.Join(dir, name), err)
		}
		res.Written = append(res.Written, pathpkg.Join(dir, name))
	}
	return nil
}

// prune removes every file under root that keep does not name, and every
// directory left empty, and returns the removed files' slash paths
// relative to root.
func prune(root *os.Root, keep map[string][]byte) ([]string, error) {
	var removed, dirs []string
	err := fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			if p != "." {
				dirs = append(dirs, p)
			}
			return nil
		}
		if _, ok := keep[p]; ok && d.Type().IsRegular() {
			return nil
		}
		removed = append(removed, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, p := range removed {
		if err := root.Remove(p); err != nil {
			return nil, err
		}
	}
	// Deepest first, so that a directory is empty once its children are
	// gone. A directory that still holds a file is kept.
	slices.Reverse(dirs)
	for _, d := range dirs {
		if entries, err := fs.ReadDir(root.FS(), d); err == nil && len(entries) == 0 {
			if err := root.Remove(d); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(removed)
	return removed, nil
}
