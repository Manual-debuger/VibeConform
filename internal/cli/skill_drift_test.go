package cli

import (
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// The drift test (spec 0054, "The drift test (#109)", Seam 2): checkSkill
// reads a skill as a file system and returns every problem it finds. The
// embedded skill must have none, and each fixture below is the embedded
// skill with one planted problem, which must be the only one found.

func TestEmbeddedSkillHasNoDrift(t *testing.T) {
	t.Parallel()
	for _, p := range checkSkill(setupskill.FS()) {
		t.Error(p)
	}
}

// skillWith is the embedded skill with edit applied to its files.
func skillWith(t *testing.T, edit func(files fstest.MapFS)) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	err := fs.WalkDir(setupskill.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(setupskill.FS(), p)
		if err != nil {
			return err
		}
		files[p] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	edit(files)
	return files
}

// appendTo adds text, as a paragraph of its own, to the end of file.
func appendTo(file, text string) func(fstest.MapFS) {
	return func(files fstest.MapFS) {
		files[file].Data = append(files[file].Data, []byte("\n"+text+"\n")...)
	}
}

// replaceIn replaces old, which must be in file, with new.
func replaceIn(t *testing.T, file, old, new string) func(fstest.MapFS) {
	return func(files fstest.MapFS) {
		f, ok := files[file]
		if !ok || !strings.Contains(string(f.Data), old) {
			t.Fatalf("%s does not contain %q", file, old)
		}
		f.Data = []byte(strings.Replace(string(f.Data), old, new, 1))
	}
}

// The drift fixtures share the reference vibe.yaml files, so the checker
// builds the Git repository for one content once, and every load of that
// content gets the same answer (spec 0056).
func TestReferenceConfigBuiltOncePerContent(t *testing.T) {
	t.Parallel()
	data, err := fs.ReadFile(setupskill.FS(), "reference/vibe-yaml/go.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// A comment no other test writes makes the content this test's own.
	f := skillFile{path: "reference/vibe-yaml/go.yaml", text: string(data) + "# " + t.Name() + "\n"}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { errs[i] = loadReferenceConfig(f) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("load %d: %v", i, err)
		}
	}
	if got := referenceConfigBuilds(f); got != 1 {
		t.Errorf("the repository for one content was built %d times, want 1", got)
	}
}

func TestSkillDriftFixtures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit func(t *testing.T) func(fstest.MapFS)
		want string // in the one problem found
	}{
		{"unknown rule ID", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "Rule X-01 is not real, and neither is V-99.")
		}, "V-99"},
		{"unknown guard rule ID", func(*testing.T) func(fstest.MapFS) {
			return appendTo("reference/rules.md", "A guard rule ID such as `git-rebase-onto` is listed here.")
		}, "git-rebase-onto"},
		{"unknown vibe command", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "Run `vibe fix --json`.")
		}, `"fix"`},
		{"unknown vibe subcommand of a command with subcommands", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "Run `vibe audit && vibe auditt`.")
		}, `"auditt"`},
		{"unknown vibe flag", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "Run `vibe audit --strict`.")
		}, "--strict"},
		{"unknown vibe command in a code block", func(*testing.T) func(fstest.MapFS) {
			return appendTo("reference/upgrade.md", "```sh\nvibe upgrade\n```")
		}, `"upgrade"`},
		{"unknown vibe command in a workflow run step", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/ci/go/audit.yml", "run: vibe audit", "run: vibe audit --verbose")
		}, "--verbose"},
		{"unknown vibe command in a GitLab script", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/ci/python/.gitlab-ci.yml", "- vibe audit", "- vibe audit --verbose")
		}, "--verbose"},
		{"unknown vibe command in a GitLab before_script", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/ci/monorepo/.gitlab-ci.yml", "  before_script:\n", "  before_script:\n    - vibe fix\n")
		}, `"fix"`},
		{"missing GitLab reference for one stack", func(*testing.T) func(fstest.MapFS) {
			return func(files fstest.MapFS) { delete(files, "reference/ci/typescript/.gitlab-ci.yml") }
		}, "reference/ci/typescript/.gitlab-ci.yml"},
		{"a GitLab job runs pnpm on a component from the root", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/ci/monorepo/.gitlab-ci.yml", "    - (cd apps/web && pnpm install --frozen-lockfile)\n", "    - pnpm --dir apps/web install --frozen-lockfile\n")
		}, "pnpm --dir apps/web"},
		{"a workflow step runs uv on a component from the root", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/ci/monorepo/ci.yml", "(cd services/worker && uv sync --frozen)", "uv sync --frozen --project=services/worker")
		}, "--project"},
		{"unknown vibe-guard command", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "Check with `vibe-guard status`.")
		}, `"status"`},
		{"unknown vibe-guard flag", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "Run `vibe-guard explain --shell bash 'ls'`.")
		}, "--shell"},
		{"missing reference file", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "See `reference/ci/go/release.yml`.")
		}, "reference/ci/go/release.yml"},
		{"missing reference file for one stack", func(*testing.T) func(fstest.MapFS) {
			return func(files fstest.MapFS) { delete(files, "reference/hooks/python/lefthook.yml") }
		}, "reference/hooks/python/lefthook.yml"},
		{"rule with no guidance", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/rules.md", "### R-03 (SHOULD)", "### R-3 (SHOULD)")
		}, "R-03"},
		{"rule with the wrong level", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/rules.md", "### C-05 (SHOULD)", "### C-05 (MUST)")
		}, "C-05"},
		{"unknown repository file", func(*testing.T) func(fstest.MapFS) {
			return appendTo("SKILL.md", "Write `.github/vibe-settings.yml`.")
		}, ".github/vibe-settings.yml"},
		{"invalid reference vibe.yaml", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/vibe-yaml/python.yaml", "verify-fast:", "verify-quick:")
		}, "reference/vibe-yaml/python.yaml"},
		{"reference vibe.yaml of version 1", func(*testing.T) func(fstest.MapFS) {
			return func(files fstest.MapFS) {
				files["reference/vibe-yaml/go.yaml"].Data = []byte("standard: prod-go\nversion: v0.9.0\n")
			}
		}, "reference/vibe-yaml/go.yaml"},
		{"invalid reference policy.json", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/agents/policy.json", `"version": 2`, `"version": 1`)
		}, "reference/agents/policy.json"},
		{"reference PreToolUse command differs from the guard's", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/agents/settings.json", "exit 2; }; vibe-guard hook", "exit 1; }; vibe-guard hook")
		}, "PreToolUse"},
		{"reference Stop command differs from the guard's", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/agents/settings.json", "vibe-guard relock 2>/dev/null; ", "")
		}, "Stop"},
		{"reference vibe.yaml declares an older contract", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/vibe-yaml/monorepo.yaml", "contract: "+contract.Implemented, "contract: 1.0.0")
		}, "reference/vibe-yaml/monorepo.yaml"},
		{"SKILL.md names an older contract", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "SKILL.md", "under contract "+contract.Implemented, "under contract 1.0.0")
		}, "SKILL.md"},
		{"a reference lint config checks the skill's directories", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "reference/lint/python/ruff.toml", `, ".agents/skills/vibeconform-setup"`, "")
		}, "reference/lint/python/ruff.toml"},
		{"the policy SKILL.md hands to the person differs from the reference", func(t *testing.T) func(fstest.MapFS) {
			return replaceIn(t, "SKILL.md", `    "rules": [],`, `    "tools": [],`)
		}, "SKILL.md"},
		{"upgrade.md does not name vibe unlock", func(*testing.T) func(fstest.MapFS) {
			return func(files fstest.MapFS) {
				f := files["reference/upgrade.md"]
				f.Data = []byte(strings.ReplaceAll(string(f.Data), "`vibe unlock`", "the unlock command"))
			}
		}, "vibe unlock"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			problems := checkSkill(skillWith(t, c.edit(t)))
			if len(problems) != 1 || !strings.Contains(problems[0], c.want) {
				t.Errorf("problems = %q, want exactly one naming %q", problems, c.want)
			}
		})
	}
}
