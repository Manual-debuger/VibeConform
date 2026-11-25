package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The contract audit's tests in this package run through the vibe audit
// command in-process, against fixture repositories (spec 0052, "Testing
// Decisions"; ADR 0028 moves the rule tests to internal/contract). A
// fixture is a tree under internal/contract/testdata, copied into a
// temporary directory and changed by the test's edits: the baseline plus
// one change, so that a test shows which change flips which result. The
// fixture builder is contracttest's, which internal/contract's tests use
// too, and the tests here call it directly: contracttest.Fixture for a
// listed fixture, contracttest.GitFixture for the tests that read git
// itself (the default branch, git failures, the golden reports), and
// contracttest.OpenRepo as the audit's repository seam. The cli TestMain
// clears GIT_DIR and its relatives, so every git here uses the fixture.

// auditResult is what one vibe audit run printed and its exit code.
type auditResult struct {
	stdout string
	stderr string
	code   int
}

// runVibeAudit runs vibe audit in-process with args after "audit",
// against the registry's rules. The runs of the execution and probe parts
// find no program: PATH is an empty directory and no bash is named, so
// that a fixture's result does not depend on what this machine has
// installed. The probe tests are in internal/contract, which runs
// contract.Audit in this process's environment.
func runVibeAudit(t *testing.T, args ...string) auditResult {
	t.Helper()
	root := newRootCmd("test", cliSeams{probeEnv: contracttest.EmptyProbeEnv(t), openRepo: contracttest.OpenRepo})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"audit"}, args...))
	err := root.Execute()
	return auditResult{stdout: out.String(), stderr: errOut.String(), code: ExitCode(err)}
}

// jsonReport is the --json report as a consumer reads it (spec 0052,
// "Report (#100)").
type jsonReport struct {
	Schema     int    `json:"schema"`
	Vibe       string `json:"vibe"`
	StaticOnly bool   `json:"static_only"`
	Verdict    string `json:"verdict"`
	Reason     string `json:"reason"`
	Contract   struct {
		Declared string `json:"declared"`
		Audited  string `json:"audited"`
	} `json:"contract"`
	Summary    map[string]map[string]int `json:"summary"`
	Rules      []jsonRule                `json:"rules"`
	CIProvider string                    `json:"ci_provider"`
	// GuardExceptionUses is a pointer so that a missing key shows.
	GuardExceptionUses *[]struct {
		Rule  string `json:"rule"`
		Match string `json:"match"`
		Case  string `json:"case"`
	} `json:"guard_exception_uses"`
}

// decodeJSONReport decodes a --json report. It is the one mirror of the
// JSON report in test code (spec 0065): the rule tests read the report
// model in internal/contract.
func decodeJSONReport(t *testing.T, out string) jsonReport {
	t.Helper()
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("no JSON report: %v\n%s", err, out)
	}
	return rep
}

type jsonRule struct {
	ID       string `json:"id"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Result   string `json:"result"`
	Parts    []struct {
		Check  string `json:"check"`
		Result string `json:"result"`
	} `json:"parts"`
	Findings []struct {
		File    string `json:"file"`
		Message string `json:"message"`
		Fix     string `json:"fix"`
	} `json:"findings"`
	Waiver *struct {
		Reason  string `json:"reason"`
		Until   string `json:"until"`
		Expired bool   `json:"expired"`
		// Unused is a pointer so that a missing key shows.
		Unused *bool `json:"unused"`
	} `json:"waiver"`
	// Required is a pointer so that a missing key shows.
	Required *bool `json:"required"`
}

// auditJSON runs vibe audit --json on a fixture and decodes the report.
func auditJSON(t *testing.T, dir string, args ...string) jsonReport {
	t.Helper()
	r := runVibeAudit(t, append([]string{"--repo-root", dir, "--json"}, args...)...)
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, r.stdout, r.stderr)
	}
	return rep
}

// rule returns the result of one rule from a report.
func (rep jsonReport) rule(t *testing.T, id string) jsonRule {
	t.Helper()
	for _, r := range rep.Rules {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("report has no rule %s", id)
	return jsonRule{}
}

// findingText joins a rule's findings for substring assertions.
func (r jsonRule) findingText() string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(f.File + ": " + f.Message + " Fix: " + f.Fix + "\n")
	}
	return b.String()
}

func assertRule(t *testing.T, rep jsonReport, id, want string, findings ...string) {
	t.Helper()
	r := rep.rule(t, id)
	if r.Result != want {
		t.Errorf("%s = %s, want %s\nfindings:\n%s", id, r.Result, want, r.findingText())
	}
	text := r.findingText()
	for _, f := range findings {
		if !strings.Contains(text, f) {
			t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
		}
	}
}

// A fixture whose files are all tracked is no Git repository: the audit
// gets its file list through the repository seam, so a table case starts
// no git (spec 0056). The audit still reads it as the baseline.
func TestListedFixtureIsAuditedWithoutGit(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline)
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("the fixture has a .git (%v), want a plain directory", err)
	}
	rep := auditJSON(t, dir, "--static")
	assertRule(t, rep, "R-02", "pass")
}

// A fixture with an untracked file, or one with a .gitignore, is a Git
// repository: only git decides what such a fixture tracks.
func TestFixtureThatNeedsGitIsARepository(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]contracttest.Edit{
		"untracked": contracttest.WithUntracked("notes.txt", "x\n"),
		"gitignore": contracttest.WithFile(".gitignore", "build/\n"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := contracttest.Fixture(t, contracttest.Baseline, edit)
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				t.Errorf("the fixture has no .git: %v", err)
			}
		})
	}
}
