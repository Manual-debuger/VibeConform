package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two skill directories vibe init --skill writes (spec 0054, "vibe
// init --skill"): Claude Code's and Codex's.
var skillDirs = []string{".claude/skills/vibeconform-setup", ".agents/skills/vibeconform-setup"}

// initSkill runs vibe init --skill in dir as a vibe of the given version.
func initSkill(t *testing.T, version, dir string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd(version)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"init", "--skill", "--repo-root", dir}, args...))
	err := root.Execute()
	return out.String(), err
}

// skillTree is every file under a skill directory, by slash path.
func skillTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestInitSkillWritesBothCopies: vibe init --skill writes the setup skill
// to both skill paths, byte-identical, with the vibe version in place of
// the placeholder, and prints each path it wrote.
func TestInitSkillWritesBothCopies(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, err := initSkill(t, "v0.12.0-alpha.1", dir)
	if err != nil {
		t.Fatalf("vibe init --skill: %v\n%s", err, out)
	}
	claude := skillTree(t, filepath.Join(dir, skillDirs[0]))
	agents := skillTree(t, filepath.Join(dir, skillDirs[1]))
	for _, f := range []string{"SKILL.md", "reference/rules.md", "reference/migrate.md", "reference/upgrade.md"} {
		if claude[f] == "" {
			t.Errorf("%s/%s was not written", skillDirs[0], f)
		}
	}
	if len(claude) != len(agents) {
		t.Errorf("%d files in %s, %d in %s", len(claude), skillDirs[0], len(agents), skillDirs[1])
	}
	for f, c := range claude {
		if agents[f] != c {
			t.Errorf("%s differs between the two copies", f)
		}
		if strings.Contains(c, "{{VIBE_VERSION}}") {
			t.Errorf("%s still holds the version placeholder", f)
		}
		for _, d := range skillDirs {
			if !strings.Contains(out, d+"/"+f) {
				t.Errorf("output does not name %s/%s:\n%s", d, f, out)
			}
		}
	}
	if !strings.Contains(claude["SKILL.md"], "v0.12.0-alpha.1") {
		t.Error("SKILL.md does not record the vibe version it came from")
	}
	if !strings.Contains(claude["reference/agents/settings.json"], "cmd/vibe-guard@v0.12.0-alpha.1, ") {
		t.Error("the reference hook command does not name the vibe version")
	}
}

// TestInitSkillVersionRule: a vibe that is not a release writes latest,
// by internal/guard's version rule (spec 0054, "Placeholders").
func TestInitSkillVersionRule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := initSkill(t, "v0.11.0-alpha.1.0.20261017172609-46246ad76fbf", dir); err != nil {
		t.Fatalf("vibe init --skill: %v\n%s", err, out)
	}
	settings := skillTree(t, filepath.Join(dir, skillDirs[0]))["reference/agents/settings.json"]
	if !strings.Contains(settings, "cmd/vibe-guard@latest, ") || strings.Contains(settings, "20261017172609") {
		t.Errorf("reference settings.json does not name latest:\n%s", settings)
	}
}

// TestInitSkillOverwritesAndPrunes: on upgrade the skill's files are
// replaced and files the embedded skill does not have are removed, with
// each removal printed.
func TestInitSkillOverwritesAndPrunes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, d := range skillDirs {
		writeTestFile(t, filepath.Join(dir, d, "SKILL.md"), "an old skill\n")
		writeTestFile(t, filepath.Join(dir, d, "reference", "stale", "old.md"), "stale\n")
	}
	out, err := initSkill(t, "v0.12.0", dir)
	if err != nil {
		t.Fatalf("vibe init --skill: %v\n%s", err, out)
	}
	for _, d := range skillDirs {
		tree := skillTree(t, filepath.Join(dir, d))
		if tree["SKILL.md"] == "an old skill\n" {
			t.Errorf("%s/SKILL.md was not overwritten", d)
		}
		if _, ok := tree["reference/stale/old.md"]; ok {
			t.Errorf("%s/reference/stale/old.md was not removed", d)
		}
		if _, err := os.Stat(filepath.Join(dir, d, "reference", "stale")); !os.IsNotExist(err) {
			t.Errorf("the emptied directory %s/reference/stale is left behind", d)
		}
		if !strings.Contains(out, "removed "+d+"/reference/stale/old.md") {
			t.Errorf("output does not report removing %s/reference/stale/old.md:\n%s", d, out)
		}
	}
}

// TestInitSkillWritesNothingElse: outside the two skill directories the
// tree is unchanged, whatever configuration the repository has (a v1
// vibe.yaml here), and no sync runs.
func TestInitSkillWritesNothingElse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "vibe.yaml"), "standard: prod-go\nversion: v1\n")
	writeTestFile(t, filepath.Join(dir, ".claude", "settings.json"), "{}\n")
	writeTestFile(t, filepath.Join(dir, ".claude", "skills", "other", "SKILL.md"), "another skill\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "skills", "vibeconform-setupx", "keep.md"), "a neighbour\n")
	before := treeHash(t, dir)
	if out, err := initSkill(t, "v0.12.0", dir); err != nil {
		t.Fatalf("vibe init --skill: %v\n%s", err, out)
	}
	after := treeHash(t, dir)
	for p, h := range after {
		if inSkillDir(p) {
			continue
		}
		if before[p] != h {
			t.Errorf("%s was written", p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("%s was removed", p)
		}
	}
}

func inSkillDir(p string) bool {
	for _, d := range skillDirs {
		if strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

// TestInitSkillRefusesPositionalArgs: --skill together with a standard
// and version is an error, and writes nothing.
func TestInitSkillRefusesPositionalArgs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := initSkill(t, "v0.12.0", dir, "prod-go", "v1"); err == nil {
		t.Fatal("vibe init --skill prod-go v1 succeeded, want an error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("the failed init wrote %d entries", len(entries))
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
