package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

// fixtureFrom builds a fixture from the tree src with edits applied and
// returns its root: a listed fixture, unless it needs git
// (contracttest.Fixture).
func fixtureFrom(t *testing.T, src string, edits ...fixtureEdit) string {
	t.Helper()
	return contracttest.Fixture(t, src, edits...)
}

// fixtureGit runs git with args in dir.
func fixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	contracttest.Git(t, dir, args...)
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

// runVibeAudit runs vibe audit in-process with args after "audit".
func runVibeAudit(t *testing.T, args ...string) auditResult {
	t.Helper()
	return runVibeAuditRules(t, nil, args...)
}

// runVibeAuditRules runs vibe audit with rules in place of the registry
// (nil for the registry). Every report it prints is watched. The runs of
// the execution and probe parts find no program: PATH is an empty
// directory and no bash is named, so that a fixture's result does not
// depend on what this machine has installed. The probe tests are in
// internal/contract, which runs contract.Audit in this process's
// environment.
func runVibeAuditRules(t *testing.T, rules []contract.Rule, args ...string) auditResult {
	t.Helper()
	return runVibeAuditSeams(t, contractSeams{rules: rules, probeEnv: emptyProbeEnv(t)}, args...)
}

// emptyProbeEnv is this process's environment with PATH set to an empty
// directory and CLAUDE_CODE_GIT_BASH_PATH removed.
func emptyProbeEnv(t *testing.T) []string {
	t.Helper()
	return contracttest.EmptyProbeEnv(t)
}

func runVibeAuditSeams(t *testing.T, seams contractSeams, args ...string) auditResult {
	t.Helper()
	seams.seen = watchReports(t)
	if seams.openRepo == nil {
		seams.openRepo = openFixtureRepo
	}
	root := newRootCmd("test", seams)
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

// A fixture whose files are all tracked is no Git repository: the audit
// gets its file list through the repository seam, so a table case starts
// no git (spec 0056). The audit still reads it as the baseline.
func TestListedFixtureIsAuditedWithoutGit(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t)
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("the fixture has a .git (%v), want a plain directory", err)
	}
	rep, _ := auditJSON(t, dir, "--static")
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

// The GitHub workflow names and helpers the GitLab rule tests still use;
// the GitHub CI rule tests moved to internal/contract (spec 0065).
const (
	ciFile    = ".github/workflows/ci.yml"
	auditFile = ".github/workflows/audit.yml"
)

// listVerify declares verify as a list of two commands.
var listVerify = entrypoints("  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")

// noVerify declares every entrypoint but verify.
var noVerify = entrypoints("  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n")

// runStaticPartCases is runRuleCases for a rule with a static part and
// other parts: under --static the others are not run, so want is the
// static part's result, and the findings are the rule's.
func runStaticPartCases(t *testing.T, id string, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep, _ := auditJSON(t, contractFixture(t, tc.edits...), "--static")
			r := rep.rule(t, id)
			got := ""
			for _, p := range r.Parts {
				if p.Check == "static" {
					got = p.Result
				}
			}
			if got != tc.want {
				t.Errorf("%s static part = %q, want %s\nfindings:\n%s", id, got, tc.want, r.findingText())
			}
			text := r.findingText()
			for _, f := range tc.findings {
				if !strings.Contains(text, f) {
					t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
				}
			}
		})
	}
}
