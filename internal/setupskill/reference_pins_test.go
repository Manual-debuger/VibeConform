package setupskill_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// The reference CI files' pins, moved here from internal/module/ci/pins
// by spec 0059 (Desired Behavior 13, Seam 2). The pin table and the
// generated CI they were checked against go with internal/module, so the
// table's versions are held here, and each action must be pinned at one
// commit across the reference files.

// refPins are the toolchain versions the reference CI files name: the
// values of internal/module/ci/pins.Current when spec 0059 removed it.
// A bump changes the reference files and this table together.
//
// Gitleaks is the release every gitleaks install step downloads for the
// secrets task (spec 0061, V-07), and the two SHA-256 fields are the
// checksums of its archives that the steps check. They hold for the
// reference CI files, this repository's workflows and the examples'
// workflows alike (TestGitleaksInstallsMatchPins).
var refPins = struct {
	Go, Task, GolangciLint, Goimports, Govulncheck, Actionlint, Node, Python string

	Gitleaks, GitleaksLinuxSHA256, GitleaksWindowsSHA256 string
}{
	Go:           "1.27.0",
	Task:         "v3.53.1",
	GolangciLint: "v2.15.0",
	Goimports:    "v0.51.0",
	Govulncheck:  "v1.8.0",
	Actionlint:   "v1.7.12",
	Node:         "22",
	Python:       "3.12",

	Gitleaks:              "8.30.1",
	GitleaksLinuxSHA256:   "551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb",
	GitleaksWindowsSHA256: "d29144deff3a68aa93ced33dddf84b7fdc26070add4aa0f4513094c8332afc4e",
}

const setupAction = "Manual-debuger/VibeConform"

var (
	// usesRE matches a uses: line pinned by commit SHA.
	usesRE = regexp.MustCompile(`(?m)uses:\s*([^@\s]+)@([0-9a-f]{40})`)
	// envRE matches a quoted version in a workflow's env: block.
	envRE = regexp.MustCompile(`(?m)^\s*([A-Z_]+_(?:VERSION|RELEASE)):\s*"([^"]*)"`)
	// installRE matches a go install with a literal version.
	installRE = regexp.MustCompile(`go install ([^@\s]+)@([^\s$]+)`)
	// setupTaskRE matches go-task/setup-task's version input.
	setupTaskRE = regexp.MustCompile(`uses:\s*go-task/setup-task@\S+[^\n]*\n\s+with:\n\s+version:\s*"?([^"\s]+)"?`)
)

// TestSkillCIMatchesPins: wherever the skill's reference CI files name a
// pinned tool, they name the version refPins holds, and every action is
// pinned at the same commit in each file, so a pin bump cannot leave one
// reference behind.
func TestSkillCIMatchesPins(t *testing.T) { //nolint:gocognit,gocyclo // baseline: pre-existing complexity
	env := map[string]string{
		"GO_VERSION":            refPins.Go,
		"TASK_VERSION":          refPins.Task,
		"GOLANGCI_LINT_VERSION": refPins.GolangciLint,
		"NODE_VERSION":          refPins.Node,
		"PYTHON_VERSION":        refPins.Python,
	}
	install := map[string]string{
		"github.com/go-task/task/v3/cmd/task":                    refPins.Task,
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint": refPins.GolangciLint,
		"golang.org/x/tools/cmd/goimports":                       refPins.Goimports,
		"golang.org/x/vuln/cmd/govulncheck":                      refPins.Govulncheck,
		"github.com/rhysd/actionlint/cmd/actionlint":             refPins.Actionlint,
	}

	skill := setupskill.FS()
	// The pattern matches .gitlab-ci.yml too: * matches a leading dot.
	files, err := fs.Glob(skill, "reference/ci/*/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 8 {
		t.Fatalf("found only %d reference CI files: %v", len(files), files)
	}
	// actions holds, per action, the commits and the files that pin it.
	actions := map[string]map[string][]string{}
	checked := map[string]int{}
	for _, file := range files {
		b, err := fs.ReadFile(skill, file)
		if err != nil {
			t.Fatal(err)
		}
		content := strings.ReplaceAll(string(b), "\r\n", "\n")
		for _, m := range usesRE.FindAllStringSubmatch(content, -1) {
			// The skill names the setup action by a placeholder commit
			// that the agent fills in from the release it installs.
			if m[1] == setupAction {
				continue
			}
			checked["actions"]++
			if actions[m[1]] == nil {
				actions[m[1]] = map[string][]string{}
			}
			actions[m[1]][m[2]] = append(actions[m[1]][m[2]], file)
		}
		for _, m := range envRE.FindAllStringSubmatch(content, -1) {
			// The GitLab references need an exact Node release (for
			// Chocolatey and nodejs.org) of the major the table pins.
			if m[1] == "NODE_RELEASE" {
				checked["env"]++
				if !strings.HasPrefix(m[2], refPins.Node+".") {
					t.Errorf("%s sets NODE_RELEASE to %q; the pin table says Node %q", file, m[2], refPins.Node)
				}
				continue
			}
			want, ok := env[m[1]]
			if !ok {
				continue
			}
			checked["env"]++
			if m[2] != want {
				t.Errorf("%s sets %s to %q; the pin table says %q", file, m[1], m[2], want)
			}
		}
		for _, m := range installRE.FindAllStringSubmatch(content, -1) {
			want, ok := install[m[1]]
			if !ok {
				t.Errorf("%s installs %s, which the pin table does not pin", file, m[1])
				continue
			}
			checked["go install"]++
			if m[2] != want {
				t.Errorf("%s installs %s@%s; the pin table says %s", file, m[1], m[2], want)
			}
		}
		for _, m := range setupTaskRE.FindAllStringSubmatch(content, -1) {
			checked["setup-task"]++
			if want := strings.TrimPrefix(refPins.Task, "v"); m[1] != want {
				t.Errorf("%s sets up Task %s; the pin table says %s", file, m[1], want)
			}
		}
	}
	for action, commits := range actions {
		if len(commits) > 1 {
			t.Errorf("%s is pinned at %d commits: %v", action, len(commits), commits)
		}
	}
	// Each kind of reference is present, so a change to the files' form
	// cannot make the check pass by matching nothing.
	for _, kind := range []string{"actions", "env", "go install", "setup-task"} {
		if checked[kind] == 0 {
			t.Errorf("no %s reference in the skill's CI files was checked", kind)
		}
	}
}

// TestSkillCIInstallsGitleaks: every reference CI file runs verify, or
// vibe audit, which runs verify in its probe copy, so each installs the
// pinned gitleaks release and checks it against its SHA-256, the Windows
// archive too where a job runs on Windows.
func TestSkillCIInstallsGitleaks(t *testing.T) {
	skill := setupskill.FS()
	files, err := fs.Glob(skill, "reference/ci/*/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "dependabot.yml") {
			continue
		}
		b, err := fs.ReadFile(skill, file)
		if err != nil {
			t.Fatal(err)
		}
		content := string(b)
		checked++
		if !installsGitleaks(content) {
			t.Errorf("%s does not install gitleaks", file)
		}
		windows := strings.Contains(content, "windows-latest") || strings.Contains(content, "saas-windows")
		if windows && !strings.Contains(content, refPins.GitleaksWindowsSHA256) {
			t.Errorf("%s runs on Windows but does not check the Windows gitleaks archive's SHA-256", file)
		}
	}
	if checked < 12 {
		t.Errorf("checked %d reference CI files, want the 12 ci.yml, audit.yml and .gitlab-ci.yml files", checked)
	}
}

var (
	// gitleaksVersionRE matches the gitleaks version a CI file names:
	// its GITLEAKS_VERSION, or a literal version in an archive's name or
	// release URL.
	gitleaksVersionRE = regexp.MustCompile(`GITLEAKS_VERSION:\s*"?([^"\s]+)"?|gitleaks_(\d[^_$]*)_|gitleaks/releases/download/v(\d[^/$]*)/`)
	// checksumRE matches a SHA-256 that is not an image digest
	// (image@sha256:...).
	checksumRE = regexp.MustCompile(`(?:^|[^:0-9a-f])([0-9a-f]{64})(?:[^0-9a-f]|$)`)
)

// installsGitleaks says whether a CI file downloads a gitleaks release.
func installsGitleaks(content string) bool {
	return strings.Contains(content, "gitleaks/releases/download/")
}

// TestGitleaksInstallsMatchPins: every copy of the gitleaks install step
// (the skill's reference CI files, this repository's workflows and the
// examples' workflows) names the gitleaks version refPins holds, checks
// the Linux archive against its SHA-256, and the Windows archive where it
// downloads one. The only checksums these files carry are gitleaks's, so
// any other SHA-256 in a file that installs gitleaks is a stale or
// mistyped pin.
func TestGitleaksInstallsMatchPins(t *testing.T) {
	files := map[string]string{}
	skill := setupskill.FS()
	refs, err := fs.Glob(skill, "reference/ci/*/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range refs {
		b, err := fs.ReadFile(skill, f)
		if err != nil {
			t.Fatal(err)
		}
		files["internal/setupskill/skill/"+f] = string(b)
	}
	repo := os.DirFS(filepath.Join("..", ".."))
	for _, pattern := range []string{".github/workflows/*.yml", "examples/*/.github/workflows/*.yml"} {
		matches, err := fs.Glob(repo, pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range matches {
			b, err := fs.ReadFile(repo, f)
			if err != nil {
				t.Fatal(err)
			}
			files[f] = string(b)
		}
	}
	installs := map[string]int{}
	for name, content := range files {
		if !installsGitleaks(content) {
			continue
		}
		installs[strings.SplitN(name, "/", 2)[0]]++
		checkGitleaksPins(t, name, content)
	}
	// Each place holds copies, so a moved or renamed file cannot make the
	// check pass by matching nothing.
	for _, place := range []string{"internal", ".github", "examples"} {
		if installs[place] == 0 {
			t.Errorf("no file under %s installs gitleaks; the check matched nothing there", place)
		}
	}
}

// checkGitleaksPins checks one CI file that installs gitleaks against
// refPins.
func checkGitleaksPins(t *testing.T, name, content string) {
	t.Helper()
	versions := 0
	for _, m := range gitleaksVersionRE.FindAllStringSubmatch(content, -1) {
		v := m[1] + m[2] + m[3]
		versions++
		if v != refPins.Gitleaks {
			t.Errorf("%s names gitleaks %s; the pin table says %s", name, v, refPins.Gitleaks)
		}
	}
	if versions == 0 {
		t.Errorf("%s installs gitleaks without naming a version", name)
	}
	pinned := map[string]bool{refPins.GitleaksLinuxSHA256: true, refPins.GitleaksWindowsSHA256: true}
	for _, m := range checksumRE.FindAllStringSubmatch(content, -1) {
		if !pinned[m[1]] {
			t.Errorf("%s checks SHA-256 %s, which is neither gitleaks archive's in the pin table", name, m[1])
		}
	}
	if !strings.Contains(content, refPins.GitleaksLinuxSHA256) {
		t.Errorf("%s does not check the Linux gitleaks archive's SHA-256", name)
	}
	if strings.Contains(content, "windows_x64.zip") && !strings.Contains(content, refPins.GitleaksWindowsSHA256) {
		t.Errorf("%s downloads the Windows gitleaks archive but does not check its SHA-256", name)
	}
}
