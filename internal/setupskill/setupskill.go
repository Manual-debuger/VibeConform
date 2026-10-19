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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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
// directories is read or written.
func Install(repoRoot, vibeVersion string) (Result, error) {
	files, err := Render(vibeVersion)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, dir := range Dirs {
		root := filepath.Join(repoRoot, filepath.FromSlash(dir))
		removed, err := prune(root, files)
		if err != nil {
			return res, err
		}
		for _, r := range removed {
			res.Removed = append(res.Removed, path.Join(dir, r))
		}
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			target := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return res, fmt.Errorf("write %s: %w", path.Join(dir, name), err)
			}
			if err := os.WriteFile(target, files[name], 0o644); err != nil { //nolint:gosec // a skill is read by everyone
				return res, fmt.Errorf("write %s: %w", path.Join(dir, name), err)
			}
			res.Written = append(res.Written, path.Join(dir, name))
		}
	}
	return res, nil
}

// prune removes every file under root that keep does not name, and every
// directory left empty, and returns the removed files' slash paths
// relative to root. A missing root has nothing to prune.
func prune(root string, keep map[string][]byte) ([]string, error) {
	if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var removed, dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." {
				dirs = append(dirs, p)
			}
			return nil
		}
		if _, ok := keep[rel]; ok && d.Type().IsRegular() {
			return nil
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		removed = append(removed, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("prune %s: %w", root, err)
	}
	// Deepest first, so that a directory is empty once its children are
	// gone. A directory that still holds a file is kept.
	slices.Reverse(dirs)
	for _, d := range dirs {
		if entries, err := os.ReadDir(d); err == nil && len(entries) == 0 {
			if err := os.Remove(d); err != nil {
				return nil, fmt.Errorf("prune %s: %w", d, err)
			}
		}
	}
	sort.Strings(removed)
	return removed, nil
}
