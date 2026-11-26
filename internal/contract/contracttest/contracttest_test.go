package contracttest_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

func TestMain(m *testing.M) {
	// A git hook running the suite exports GIT_DIR; every git the tests
	// start would then use this repository instead of the fixture.
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		_ = os.Unsetenv(v)
	}
	os.Exit(m.Run())
}

// ConfigWith with no change is the baseline tree's vibe.yaml, so a test
// that edits one field changes only that field.
func TestConfigWithNoChangeIsTheBaselineVibeYAML(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(contracttest.Baseline, "vibe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := contracttest.ConfigWith(); got != string(data) {
		t.Errorf("ConfigWith() =\n%s\nthe baseline's vibe.yaml is\n%s", got, data)
	}
}

func repoFiles(t *testing.T, dir string) []string {
	t.Helper()
	repo, err := contracttest.OpenRepo(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo.Files()
}

func hasGit(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// A fixture whose files are all tracked is listed, not a Git repository;
// OpenRepo lists its files, with testdata's dot-claude as .claude and
// the edits applied.
func TestFixtureIsListed(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline,
		contracttest.WithFile("docs/new.md", "x\n"), contracttest.WithoutFile("CLAUDE.md"))
	if hasGit(dir) {
		t.Fatal("a fixture with every file tracked is a Git repository")
	}
	repo, err := contracttest.OpenRepo(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".claude/settings.json", "docs/new.md", "AGENTS.md"} {
		if !repo.HasFile(f) {
			t.Errorf("the listed fixture lacks %s: %q", f, repo.Files())
		}
	}
	if repo.HasFile("CLAUDE.md") {
		t.Errorf("the listed fixture still has the removed CLAUDE.md")
	}
}

// A .gitignore or GitFixture make a Git repository whose files git
// lists.
func TestFixtureThatNeedsGitIsARepository(t *testing.T) {
	t.Parallel()
	for name, build := range map[string]func(t *testing.T) string{
		"gitignore": func(t *testing.T) string {
			return contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile(".gitignore", "build/\n"))
		},
		"GitFixture": func(t *testing.T) string { return contracttest.GitFixture(t, contracttest.Baseline) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := build(t)
			if !hasGit(dir) {
				t.Fatal("the fixture has no .git")
			}
			if !slices.Contains(repoFiles(t, dir), "AGENTS.md") {
				t.Error("git does not track the baseline's AGENTS.md")
			}
		})
	}
}

// GitFixture and Git run git isolated from the person's configuration
// (internal/testkit): neither a GIT_DIR in the environment nor the
// person's global init.defaultBranch reaches the fixture.
func TestGitFixtureIgnoresThePersonsGitConfiguration(t *testing.T) {
	person := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(person, []byte("[init]\n\tdefaultBranch = person\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.git")
	t.Setenv("GIT_CONFIG_GLOBAL", person)
	t.Setenv("GIT_DIR", elsewhere)

	dir := contracttest.GitFixture(t, contracttest.Baseline)
	head := func() string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
		if err != nil {
			t.Fatalf("the fixture has no .git/HEAD: %v", err)
		}
		return strings.TrimSpace(string(data))
	}
	if got := head(); got != "ref: refs/heads/main" {
		t.Errorf("the fixture's HEAD is %q, want refs/heads/main", got)
	}
	contracttest.Git(t, dir, "symbolic-ref", "HEAD", "refs/heads/trunk")
	if got := head(); got != "ref: refs/heads/trunk" {
		t.Errorf("after Git, the fixture's HEAD is %q, want refs/heads/trunk", got)
	}
	if _, err := os.Stat(elsewhere); err == nil {
		t.Errorf("git wrote the person's GIT_DIR %s", elsewhere)
	}
}

// An untracked file makes a Git repository; the file is in the working
// tree but git does not track it.
func TestUntrackedFileIsNotTracked(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithUntracked("notes.txt", "x\n"))
	if !hasGit(dir) {
		t.Fatal("the fixture has no .git")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Errorf("the untracked file is not in the working tree: %v", err)
	}
	files := repoFiles(t, dir)
	if slices.Contains(files, "notes.txt") || !slices.Contains(files, "AGENTS.md") {
		t.Errorf("git tracks %q, want AGENTS.md and not notes.txt", files)
	}
}

// Under moves edits under a directory.
func TestUnder(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, t.TempDir(),
		contracttest.Under("svc/api", contracttest.WithFile("go.mod", "module api\n"))...)
	if got := repoFiles(t, dir); len(got) != 1 || got[0] != "svc/api/go.mod" {
		t.Errorf("files = %q, want [svc/api/go.mod]", got)
	}
}

// The baseline passes R-02, and RunRuleCases reads a case's edits and
// its findings.
func TestRunRuleCases(t *testing.T) {
	t.Parallel()
	contracttest.RunRuleCases(t, "R-02", []contracttest.RuleCase{
		{Name: "baseline", Want: contract.ResultPass},
		{Name: "AGENTS.md missing", Want: contract.ResultFail,
			Edits:    []contracttest.Edit{contracttest.WithoutFile("AGENTS.md")},
			Findings: []string{"AGENTS.md: ", "create"}},
	})
}

// RunRuleCases and RunPartCases run each check once in each case's
// subtest, on that case's report.
func TestRunCasesChecks(t *testing.T) {
	t.Parallel()
	cases := []contracttest.RuleCase{
		{Name: "baseline", Want: contract.ResultPass},
		{Name: "AGENTS.md missing", Want: contract.ResultFail, Edits: []contracttest.Edit{contracttest.WithoutFile("AGENTS.md")}},
	}
	runners := map[string]func(*testing.T, contracttest.CaseCheck){
		"RunRuleCases": func(t *testing.T, c contracttest.CaseCheck) { contracttest.RunRuleCases(t, "R-02", cases, c) },
		"RunPartCases": func(t *testing.T, c contracttest.CaseCheck) {
			contracttest.RunPartCases(t, "R-02", contract.Static, cases, c)
		},
	}
	for name, run := range runners {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			seen := map[string][]contract.Result{}
			// t.Run returns once the parallel case subtests have finished.
			t.Run("cases", func(t *testing.T) {
				run(t, func(t *testing.T, tc contracttest.RuleCase, rep *contract.Report) {
					mu.Lock()
					defer mu.Unlock()
					seen[tc.Name] = append(seen[tc.Name], contracttest.RuleResult(t, rep, "R-02").Result)
				})
			})
			want := map[string][]contract.Result{"baseline": {contract.ResultPass}, "AGENTS.md missing": {contract.ResultFail}}
			if !maps.EqualFunc(seen, want, slices.Equal[[]contract.Result]) {
				t.Errorf("the check saw %v, want %v", seen, want)
			}
		})
	}
}

// Rules keeps registry order, whatever order the IDs are given in.
func TestRulesKeepsRegistryOrder(t *testing.T) {
	t.Parallel()
	var want []string
	for _, r := range contract.Rules() {
		if r.ID == "V-04" || r.ID == "R-02" {
			want = append(want, r.ID)
		}
	}
	for _, order := range [][]string{{"V-04", "R-02"}, {"R-02", "V-04"}} {
		var got []string
		for _, r := range contracttest.Rules(t, order...) {
			got = append(got, r.ID)
		}
		if !slices.Equal(got, want) {
			t.Errorf("Rules(%q) = %q, want the registry's order %q", order, got, want)
		}
	}
}

// EmptyProbeEnv's PATH is an empty directory, and no bash is named.
func TestEmptyProbeEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_GIT_BASH_PATH", "/some/bash")
	env := contracttest.EmptyProbeEnv(t)
	var paths []string
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		switch {
		case strings.EqualFold(name, "CLAUDE_CODE_GIT_BASH_PATH"):
			t.Errorf("the environment names bash: %s", kv)
		case strings.EqualFold(name, "PATH"):
			paths = append(paths, value)
		}
	}
	if len(paths) != 1 {
		t.Fatalf("PATH is set %d times, want once: %q", len(paths), paths)
	}
	entries, err := os.ReadDir(paths[0])
	if err != nil || len(entries) != 0 {
		t.Errorf("PATH %s is not an empty directory: %v, %d entries", paths[0], err, len(entries))
	}
}

// StandIn makes every part of the rules it names decide as told, with a
// finding that names a file and a fix, and leaves the others as they are.
func TestStandIn(t *testing.T) {
	t.Parallel()
	rules := contracttest.StandIn(nil, contract.ResultUndecided, "V-01", "R-03")
	if len(rules) != len(contract.Rules()) {
		t.Fatalf("StandIn(nil) has %d rules, want the registry's %d", len(rules), len(contract.Rules()))
	}
	rules = contracttest.StandIn(rules, contract.ResultFail, "R-03")
	want := map[string]contract.Result{"V-01": contract.ResultUndecided, "R-03": contract.ResultFail}
	for _, r := range rules {
		w, ok := want[r.ID]
		if !ok {
			continue
		}
		for _, p := range r.Parts {
			got := p.Run(nil)
			if got.Result != w || len(got.Findings) != 1 || got.Findings[0].File == "" || got.Findings[0].Fix == "" {
				t.Errorf("%s %s part = %+v, want %s with a finding naming a file and a fix", r.ID, p.Kind, got, w)
			}
		}
	}
}

// StandIn returns a new slice: the rules it is given still decide as
// before, so a test can build two rule sets from one.
func TestStandInLeavesItsInputUntouched(t *testing.T) {
	t.Parallel()
	in := contracttest.StandIn(nil, contract.ResultPass, "V-01")
	out := contracttest.StandIn(in, contract.ResultFail, "V-01")
	result := func(rules []contract.Rule) contract.Result {
		t.Helper()
		for _, r := range rules {
			if r.ID == "V-01" {
				return r.Parts[0].Run(nil).Result
			}
		}
		t.Fatal("no V-01")
		return ""
	}
	if got := result(in); got != contract.ResultPass {
		t.Errorf("the input's V-01 = %s after a second StandIn, want pass", got)
	}
	if got := result(out); got != contract.ResultFail {
		t.Errorf("the output's V-01 = %s, want fail", got)
	}
}

// StandIn keeps R-01's execution part the last part, so the stand-in
// rules run in the registry's order and pass the runner's validation.
func TestStandInKeepsTheLastPart(t *testing.T) {
	t.Parallel()
	for _, r := range contracttest.StandIn(nil, contract.ResultFail, "R-01") {
		if r.ID != "R-01" {
			continue
		}
		for _, p := range r.Parts {
			if p.Kind != contract.Execution {
				continue
			}
			if p.Run != nil || p.RunLast == nil {
				t.Fatalf("R-01's execution part sets Run %t and RunLast %t, want only RunLast", p.Run != nil, p.RunLast != nil)
			}
			if got := p.RunLast(nil, contract.Earlier{}); got.Result != contract.ResultFail {
				t.Errorf("R-01's execution part = %s, want fail", got.Result)
			}
			return
		}
	}
	t.Fatal("no execution part of R-01")
}

// RuleIDs lists a level's rules in registry order, without those named.
func TestRuleIDs(t *testing.T) {
	t.Parallel()
	all := contracttest.RuleIDs("")
	if len(all) != len(contract.Rules()) || all[0] != contract.Rules()[0].ID {
		t.Errorf(`RuleIDs("") = %q, want every rule in registry order`, all)
	}
	must := contracttest.RuleIDs(contract.Must, "V-04")
	if slices.Contains(must, "V-04") || !slices.Contains(must, "V-01") || slices.Contains(must, "R-02") {
		t.Errorf("RuleIDs(MUST, V-04) = %q, want the MUST rules but V-04", must)
	}
	if got := contracttest.RuleIDs(contract.Should, "R-02"); slices.Contains(got, "R-02") || !slices.Contains(got, "R-03") {
		t.Errorf("RuleIDs(SHOULD, R-02) = %q, want the SHOULD rules but R-02", got)
	}
}

// AgentsConfig replaces only the agents field of the baseline vibe.yaml.
func TestAgentsConfig(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.AgentsConfig("[codex]"))
	data, err := os.ReadFile(filepath.Join(dir, "vibe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: "agents: [codex]\n"}); string(data) != want {
		t.Errorf("vibe.yaml =\n%s\nwant\n%s", data, want)
	}
}
