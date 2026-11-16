package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The contract audit's tests in this package run through the vibe audit
// command in-process, against fixture repositories (spec 0052, "Testing
// Decisions"; ADR 0028 moves the rule tests to internal/contract). A
// fixture is a tree under internal/contract/testdata, copied into a
// temporary directory and changed by the test's edits: the baseline plus
// one change, so that a test shows which change flips which result. The
// fixture builder is contracttest's, which internal/contract's tests use
// too; the helpers below are its names in this package. The cli TestMain
// clears GIT_DIR and its relatives, so every git here uses the fixture.

// baselineFixture is the tree every contract fixture starts from.
var baselineFixture = contracttest.Baseline

// fixtureEdit changes a fixture before or after git add.
type fixtureEdit = contracttest.Edit

// withFile writes path (slash separated) with content before git add, so
// the file is tracked.
func withFile(path, content string) fixtureEdit { return contracttest.WithFile(path, content) }

// withoutFile removes path from the fixture before git add.
func withoutFile(path string) fixtureEdit { return contracttest.WithoutFile(path) }

// withUntracked writes path after git add, so the file exists in the
// working tree but is not tracked.
func withUntracked(path, content string) fixtureEdit {
	return contracttest.WithUntracked(path, content)
}

// contractFixture builds a fixture from the baseline with edits applied
// and returns its root. See contracttest.Fixture for when it is a Git
// repository.
func contractFixture(t *testing.T, edits ...fixtureEdit) string {
	t.Helper()
	return contracttest.Fixture(t, baselineFixture, edits...)
}

// contractGitFixture builds a fixture from the baseline that is always a
// Git repository, for the tests that read git itself: the default branch,
// git failures, the golden reports.
func contractGitFixture(t *testing.T, edits ...fixtureEdit) string {
	t.Helper()
	return contracttest.GitFixture(t, baselineFixture, edits...)
}

// fixtureGit runs git with args in dir.
func fixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	contracttest.Git(t, dir, args...)
}

// dependabot is a .github/dependabot.yml with the given updates entries,
// each "ecosystem directory-key directory-value". vibe init's tests use
// it; the D-02 tests in internal/contract have their own.
func dependabot(entries ...string) fixtureEdit {
	var b strings.Builder
	b.WriteString("version: 2\nupdates:\n")
	for _, e := range entries {
		f := strings.SplitN(e, " ", 3)
		b.WriteString("  - package-ecosystem: " + f[0] + "\n    " + f[1] + ": " + f[2] + "\n    schedule:\n      interval: weekly\n")
	}
	return withFile(".github/dependabot.yml", b.String())
}

// openFixtureRepo is the audit's repository seam in tests: a listed
// fixture gets its file list, any other directory contract.OpenRepo.
var openFixtureRepo contract.RepoOpener = contracttest.OpenRepo

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
	root := newRootCmd("test", contractSeams{probeEnv: contracttest.EmptyProbeEnv(t), openRepo: openFixtureRepo})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"audit"}, args...))
	err := root.Execute()
	return auditResult{stdout: out.String(), stderr: errOut.String(), code: ExitCode(err)}
}

// treeHash hashes every file under dir, .git included, by path and
// content.
func treeHash(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// section is one top-level field of a fixture vibe.yaml, with its text.
type section struct {
	key  string
	body string
}

// configWith is the baseline vibe.yaml with fields replaced in place (an
// empty body removes the field) and new fields appended in order.
func configWith(changes ...section) string {
	out := make([]contracttest.Section, len(changes))
	for i, c := range changes {
		out[i] = contracttest.Section{Key: c.key, Body: c.body}
	}
	return contracttest.ConfigWith(out...)
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
	dir := contractFixture(t)
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
	for name, edit := range map[string]fixtureEdit{
		"untracked": withUntracked("notes.txt", "x\n"),
		"gitignore": withFile(".gitignore", "build/\n"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := contractFixture(t, edit)
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				t.Errorf("the fixture has no .git: %v", err)
			}
		})
	}
}
