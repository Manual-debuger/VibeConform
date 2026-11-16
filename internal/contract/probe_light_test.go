package contract_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// One light test per execution and probe part through contract.Audit
// (spec 0053, acceptance criterion 3 as amended on 2026-10-18). The
// fixtures with real tools and guards are in internal/contract/probetest
// and guardprobe_test.go; these cases need no tool: each runs only the
// named rules with an empty PATH and no bash, so each decides without
// starting a program. D-01 (TestRuleD01), A-02 (TestRuleA02Reporting)
// and A-08 (TestRuleA08Reporting) have theirs beside the reports they
// check.
func TestProbePartsLight(t *testing.T) {
	t.Parallel()
	noTests := contracttest.WithoutFile("calc/calc_test.go")
	cases := []struct {
		name     string
		rules    []string
		edits    []contracttest.Edit
		id       string
		kind     contract.PartKind
		want     contract.Result
		findings []string
	}{
		{"V-01 execution cannot decide without go", []string{"V-01"}, nil,
			"V-01", contract.Execution, contract.ResultUndecided, []string{"`go` is not installed on this machine"}},
		{"V-01 probe fails for a component with no tests", []string{"V-01"}, []contracttest.Edit{noTests},
			"V-01", contract.Probe, contract.ResultFail, []string{"component . has no tests"}},
		{"V-02 probe cannot decide with nowhere to plant", []string{"V-02"}, []contracttest.Edit{noTests},
			"V-02", contract.Probe, contract.ResultUndecided, nil},
		{"V-03 execution cannot decide without go", []string{"V-03"}, nil,
			"V-03", contract.Execution, contract.ResultUndecided, []string{"`go` is not installed on this machine"}},
		{"A-03 probe fails with no PreToolUse hook", []string{"A-03"}, nil,
			"A-03", contract.Probe, contract.ResultFail, []string{
				"with PATH reduced to the operating system's directories, ",
				"with .claude/hooks/policy.json replaced by `{`, ",
				"deny cases were not refused",
			}},
		{"A-05 probe fails with no Stop hook", []string{"A-05"}, nil,
			"A-05", contract.Probe, contract.ResultFail, []string{"has no Stop hook"}},
		{"A-07 execution passes with no SessionStart hook", []string{"A-07"},
			[]contracttest.Edit{contracttest.WithFile(".claude/settings.json", "{}\n")},
			"A-07", contract.Execution, contract.ResultPass, nil},
		{"R-01 execution fails when A-02's deny cases are not refused", []string{"R-01", "A-02"}, nil,
			"R-01", contract.Execution, contract.ResultFail, []string{"no PreToolUse hook matches Bash"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rep := contracttest.Audit(t, probeFixture(t, c.edits...), contract.Options{
				Rules: contracttest.Rules(t, c.rules...),
				Probe: contract.ProbeOptions{Env: contracttest.EmptyProbeEnv(t)},
			})
			contracttest.AssertPart(t, rep, c.id, c.kind, c.want, c.findings...)
		})
	}
}

// TestProbeIgnoresAnInvalidGitBashPath is a regression test: Claude Code
// ignores a CLAUDE_CODE_GIT_BASH_PATH that does not exist and finds Git
// Bash itself (https://code.claude.com/docs/en/env-vars), so the audit
// does too. Before, every hook part was undecided.
func TestProbeIgnoresAnInvalidGitBashPath(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil && runtime.GOOS != "windows" {
		t.Skip("bash is not on PATH")
	}
	dir := probeFixture(t, probeSessionStartHooks("startup", "exit 0"))
	env := append(contracttest.ProbeEnvWithPath(os.Getenv("PATH")),
		"CLAUDE_CODE_GIT_BASH_PATH="+filepath.Join(t.TempDir(), "missing", "bash.exe"))
	rep := contracttest.Audit(t, dir, contract.Options{
		Rules: contracttest.Rules(t, "A-07"),
		Probe: contract.ProbeOptions{Env: env},
	})
	contracttest.AssertPart(t, rep, "A-07", contract.Execution, contract.ResultPass)
}
