package cli

import (
	"encoding/json"
	"maps"
	pathpkg "path"
	"regexp"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/setupskill"
	"gopkg.in/yaml.v3"
)

// The reference implementations pass the static audit (spec 0054, "The
// drift test (#109)"; spec 0055, Seam 2): for each stack and CI provider,
// a fixture repository assembled from the skill's reference files around
// a minimal component with one test has no failing MUST rule under vibe
// audit --static --json. The static audit runs no tool.

// minimalComponents are the source files of a minimal component with one
// test, by language, relative to the component's directory.
var minimalComponents = map[string]map[string]string{
	"go": {
		"go.mod":      "module example.com/app\n\ngo 1.24\n",
		"app.go":      "package app\n\n// Add returns a + b.\nfunc Add(a, b int) int { return a + b }\n",
		"app_test.go": "package app\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n",
	},
	"python": {
		"pyproject.toml":    "[project]\nname = \"app\"\nversion = \"0.1.0\"\nrequires-python = \">=3.12\"\n",
		"uv.lock":           "version = 1\nrequires-python = \">=3.12\"\n",
		"app/__init__.py":   "def add(a: int, b: int) -> int:\n    return a + b\n",
		"tests/test_app.py": "from app import add\n\n\ndef test_add() -> None:\n    assert add(1, 2) == 3\n",
	},
	"typescript": {
		"package.json":    "{\n  \"name\": \"app\",\n  \"private\": true,\n  \"type\": \"module\"\n}\n",
		"pnpm-lock.yaml":  "lockfileVersion: '9.0'\n",
		"src/add.ts":      "export function add(a: number, b: number): number {\n  return a + b;\n}\n",
		"src/add.test.ts": "import { add } from \"./add\";\n\nif (add(1, 2) !== 3) {\n  throw new Error(\"add(1, 2) !== 3\");\n}\n",
	},
}

// referenceRepo is the fixture repository of stack on provider: every
// reference file the stack and provider use at its place, with the
// version placeholder rendered, and a minimal component for each
// component.
func referenceRepo(t *testing.T, stack, provider string) map[string]string {
	t.Helper()
	rendered, err := setupskill.Render("v0.12.0-alpha.1")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, ref := range referenceFiles(setupskill.FS()) {
		if target, ok := referenceTarget(stack, provider, ref); ok {
			files[target] = string(rendered[ref])
		}
	}
	if stack == "monorepo" {
		for lang, dir := range monorepoComponents {
			for name, content := range minimalComponents[lang] {
				files[dir+"/"+name] = content
			}
		}
	} else {
		maps.Copy(files, minimalComponents[stack])
	}
	return files
}

// staticReport is the part of a --static --json report these tests read.
type staticReport struct {
	CIProvider string `json:"ci_provider"`
	Rules      []struct {
		ID     string          `json:"id"`
		Level  contract.Level  `json:"level"`
		Result contract.Result `json:"result"`
		Parts  []struct {
			Check  contract.PartKind `json:"check"`
			Result contract.Result   `json:"result"`
		} `json:"parts"`
		Findings []struct {
			File    string `json:"file"`
			Message string `json:"message"`
		} `json:"findings"`
	} `json:"rules"`
}

// auditReference runs the static audit on the files of a reference
// repository.
func auditReference(t *testing.T, files map[string]string) staticReport {
	t.Helper()
	var edits []fixtureEdit
	for name, content := range files {
		edits = append(edits, withFile(name, content))
	}
	dir := fixtureFrom(t, t.TempDir(), edits...)
	res := runVibeAudit(t, "--static", "--json", "--repo-root", dir)
	var report staticReport
	if err := json.Unmarshal([]byte(res.stdout), &report); err != nil {
		t.Fatalf("the report is not JSON: %v\nstdout: %s\nstderr: %s", err, res.stdout, res.stderr)
	}
	if len(report.Rules) == 0 {
		t.Fatalf("the report has no rules\nstdout: %s", res.stdout)
	}
	return report
}

// findings joins the findings of a rule of the report.
func (r staticReport) findings(id string) (contract.Result, string) {
	for _, rule := range r.Rules {
		if rule.ID == id {
			var msgs []string
			for _, f := range rule.Findings {
				msgs = append(msgs, f.File+": "+f.Message)
			}
			return rule.Result, strings.Join(msgs, "; ")
		}
	}
	return "", "(no such rule)"
}

// staticPart is the result of the static part of a rule of the report.
func (r staticReport) staticPart(id string) contract.Result {
	for _, rule := range r.Rules {
		if rule.ID != id {
			continue
		}
		for _, p := range rule.Parts {
			if p.Check == contract.Static {
				return p.Result
			}
		}
	}
	return ""
}

// withStopCommand returns the files with the Stop hook of
// .claude/settings.json replaced by one that runs command.
func withStopCommand(t *testing.T, files map[string]string, command string) map[string]string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal([]byte(files[".claude/settings.json"]), &settings); err != nil {
		t.Fatalf(".claude/settings.json: %v", err)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal(".claude/settings.json has no hooks")
	}
	hooks["Stop"] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	changed := maps.Clone(files)
	changed[".claude/settings.json"] = string(out) + "\n"
	return changed
}

// TestStaticAuditReadsACdPrefixedHookCommand: the hook prefix of spec 0058
// (#133) keeps the reference conformant. The static audit splits a hook
// command into simple commands, so `cd "…" && task verify:fast` still
// calls verify-fast for A-05 and a `vibe` call after the cd still counts
// for R-01, while `task -d … verify:fast` is not the literal call A-05
// reads (spec 0057, constraint 1).
func TestStaticAuditReadsACdPrefixedHookCommand(t *testing.T) {
	t.Parallel()
	base := referenceRepo(t, "monorepo", providerGitHub)
	cases := []struct {
		name, command, rule string
		want                contract.Result
	}{
		{"cd prefix calls verify-fast", `cd "${CLAUDE_PROJECT_DIR:-.}" && task verify:fast 1>&2 </dev/null`,
			"A-05", contract.ResultPass},
		{"task -d is not the literal call", `task -d "${CLAUDE_PROJECT_DIR:-.}" verify:fast 1>&2 </dev/null`,
			"A-05", contract.ResultFail},
		{"cd prefix with no vibe", `cd "${CLAUDE_PROJECT_DIR:-.}" && task verify:fast 1>&2 </dev/null`,
			"R-01", contract.ResultPass},
		{"vibe after a cd prefix is a vibe call", `cd "${CLAUDE_PROJECT_DIR:-.}" && vibe audit --static; task verify:fast`,
			"R-01", contract.ResultFail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			report := auditReference(t, withStopCommand(t, base, c.command))
			if got := report.staticPart(c.rule); got != c.want {
				_, msgs := report.findings(c.rule)
				t.Errorf("%s static part = %q, want %q: %s", c.rule, got, c.want, msgs)
			}
		})
	}
}

// gitlabDecided are the rules a GitLab reference repository must pass
// under the static audit, MUST and SHOULD: the CI rules the GitLab
// reference decides (C-03 is not applicable on GitLab CI), its update bot,
// its scanners and its merge request template.
var gitlabDecided = []string{"C-01", "C-02", "C-04", "C-05", "D-02", "D-03", "R-03"}

func TestReferenceImplementationsPassTheStaticAudit(t *testing.T) {
	t.Parallel()
	for _, provider := range referenceProviders {
		for _, stack := range referenceStacks {
			t.Run(provider+"/"+stack, func(t *testing.T) {
				t.Parallel()
				report := auditReference(t, referenceRepo(t, stack, provider))
				for _, r := range report.Rules {
					if r.Level == contract.Must && r.Result == contract.ResultFail {
						_, msgs := report.findings(r.ID)
						t.Errorf("MUST rule %s fails: %s", r.ID, msgs)
					}
				}
				if provider != providerGitLab {
					return
				}
				if report.CIProvider != "gitlab_ci" {
					t.Errorf("ci_provider = %q, want gitlab_ci", report.CIProvider)
				}
				for _, id := range gitlabDecided {
					if got, msgs := report.findings(id); got != contract.ResultPass {
						t.Errorf("%s = %s, want pass: %s", id, got, msgs)
					}
				}
				if got, msgs := report.findings("C-03"); got != contract.ResultNotApplicable {
					t.Errorf("C-03 = %s, want not_applicable: %s", got, msgs)
				}
			})
		}
	}
}

// githubOnlyTools are tools that check GitHub Actions only, which the
// GitLab reference CI does not install.
var githubOnlyTools = []string{"actionlint"}

// TestReferenceVerifyCallsOnlyTheProvidersTools: the verify task of a
// reference repository on GitLab CI runs no GitHub-only tool, so the
// GitLab reference CI's task verify does not fail on a missing
// actionlint (the gitlab.com run of spec 0055), while the GitHub one
// still runs it where the Taskfile names it.
func TestReferenceVerifyCallsOnlyTheProvidersTools(t *testing.T) {
	t.Parallel()
	for _, provider := range referenceProviders {
		for _, stack := range referenceStacks {
			t.Run(provider+"/"+stack, func(t *testing.T) {
				files := referenceRepo(t, stack, provider)
				ran := verifyCommands(t, files)
				for _, tool := range githubOnlyTools {
					named := false
					for name, content := range files {
						if strings.HasSuffix(name, "Taskfile.yml") && strings.Contains(content, tool) {
							named = true
						}
					}
					switch {
					case provider == providerGitLab && ran[tool]:
						t.Errorf("task verify runs %s, which only checks GitHub Actions and the GitLab CI does not install", tool)
					case provider == providerGitHub && named && !ran[tool]:
						t.Errorf("task verify does not run %s, which the Taskfile names", tool)
					}
				}
			})
		}
	}
}

// taskfile is the part of a Taskfile.yml verifyCommands reads.
type taskfile struct {
	Includes map[string]struct {
		Taskfile string `yaml:"taskfile"`
		Dir      string `yaml:"dir"`
	} `yaml:"includes"`
	Tasks map[string]struct {
		Cmds   []yaml.Node `yaml:"cmds"`
		Deps   []yaml.Node `yaml:"deps"`
		Status []string    `yaml:"status"`
	} `yaml:"tasks"`
}

// skipStatus is a status command that holds when a path is missing.
var skipStatus = regexp.MustCompile(`^(?:test|\[) ! -[de] (\S+?)(?: \])?$`)

// verifyCommands are the commands, by their first word, that task verify
// of the repository files runs. A task is skipped when its status holds:
// only status commands of the form "test ! -d <path>" are evaluated,
// against files; any other status is taken not to hold.
func verifyCommands(t *testing.T, files map[string]string) map[string]bool {
	t.Helper()
	ran := map[string]bool{}
	load := func(path string) taskfile {
		var tf taskfile
		content, ok := files[path]
		if !ok {
			t.Fatalf("%s is missing", path)
		}
		if err := yaml.Unmarshal([]byte(content), &tf); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return tf
	}
	missing := func(dir, p string) bool {
		p = pathpkg.Join(dir, p)
		for name := range files {
			if name == p || strings.HasPrefix(name, p+"/") {
				return false
			}
		}
		return true
	}
	var run func(dir, file, name string)
	run = func(dir, file, name string) {
		tf := load(file)
		if ns, rest, ok := strings.Cut(name, ":"); ok {
			if inc, ok := tf.Includes[ns]; ok {
				run(pathpkg.Join(dir, inc.Dir), pathpkg.Clean(pathpkg.Join(pathpkg.Dir(file), inc.Taskfile)), rest)
				return
			}
		}
		task, ok := tf.Tasks[name]
		if !ok {
			t.Fatalf("%s has no task %q", file, name)
		}
		skipped := len(task.Status) > 0
		for _, s := range task.Status {
			m := skipStatus.FindStringSubmatch(strings.TrimSpace(s))
			if m == nil || !missing(dir, m[1]) {
				skipped = false
			}
		}
		if skipped {
			return
		}
		for _, n := range task.Deps {
			if n.Kind == yaml.ScalarNode {
				run(dir, file, n.Value)
				continue
			}
			var ref struct {
				Task string `yaml:"task"`
			}
			if err := n.Decode(&ref); err != nil {
				t.Fatalf("%s: task %s: %v", file, name, err)
			}
			run(dir, file, ref.Task)
		}
		for _, n := range task.Cmds {
			cmd := n.Value
			if n.Kind == yaml.MappingNode {
				var ref struct {
					Task string `yaml:"task"`
					Cmd  string `yaml:"cmd"`
				}
				if err := n.Decode(&ref); err != nil {
					t.Fatalf("%s: task %s: %v", file, name, err)
				}
				if ref.Task != "" {
					run(dir, file, ref.Task)
					continue
				}
				cmd = ref.Cmd
			}
			for _, simple := range commandSeparators.Split(cmd, -1) {
				if words := strings.Fields(simple); len(words) > 0 {
					ran[words[0]] = true
				}
			}
		}
	}
	run(".", "Taskfile.yml", "verify")
	return ran
}

// TestGitLabReferencePlantedProblems: each GitLab reference CI with one
// planted problem fails the rule that problem breaks (spec 0055, Seam 2),
// so the static audit test above would catch the reference drifting that
// way.
func TestGitLabReferencePlantedProblems(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, old, new string
		rule, finding  string
	}{
		{name: "an image pinned by tag only", old: "@sha256:", new: "@sha256-removed:",
			rule: "C-02", finding: "is not pinned by digest"},
		{name: "an unpinned GitLab template include", old: "\nworkflow:",
			new:  "\ninclude:\n  - template: Jobs/Dependency-Scanning.gitlab-ci.yml\n\nworkflow:",
			rule: "C-02", finding: "cannot be pinned"},
		{name: "a tool installed at @latest", old: "\n  script:\n", new: "\n  script:\n    - go install example.com/tool@latest\n",
			rule: "C-02", finding: "@latest"},
		{name: "the verify jobs gated by changes:", old: "    - if: $CI_PIPELINE_SOURCE == \"merge_request_event\"\n",
			new:  "    - if: $CI_PIPELINE_SOURCE == \"merge_request_event\"\n      changes: [\"**/*\"]\n",
			rule: "C-01", finding: "merge request pipelines"},
		{name: "the audit job also calls verify", old: "    - vibe audit\n", new: "    - task verify\n    - vibe audit\n",
			rule: "C-04", finding: "also calls the verify command"},
		{name: "a job other than the audit calls vibe", old: "    - task verify\n", new: "    - task verify\n    - vibe audit --static\n",
			rule: "R-01", finding: "vibe"},
	}
	for _, stack := range referenceStacks {
		for _, c := range cases {
			t.Run(stack+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				files := referenceRepo(t, stack, providerGitLab)
				ci := files[".gitlab-ci.yml"]
				if !strings.Contains(ci, c.old) {
					t.Fatalf("the %s reference .gitlab-ci.yml does not contain %q", stack, c.old)
				}
				files[".gitlab-ci.yml"] = strings.ReplaceAll(ci, c.old, c.new)
				report := auditReference(t, files)
				got, msgs := report.findings(c.rule)
				if got != contract.ResultFail || !strings.Contains(msgs, c.finding) {
					t.Errorf("%s = %s with %q, want fail naming %q", c.rule, got, msgs, c.finding)
				}
			})
		}
	}
}
