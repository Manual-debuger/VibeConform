package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// One light test per execution and probe part through the vibe audit
// seam (spec 0053, acceptance criterion 3 as amended on 2026-10-18). The
// fixtures with real tools and guards are in internal/contract/probetest
// and internal/contract (guardprobe_test.go); these cases need no tool:
// auditJSONRules runs only the named rules with an empty PATH and no
// bash, so each decides without starting a program. D-01 (TestRuleD01),
// A-02 (TestRuleA02Reporting) and A-08 (TestRuleA08Reporting) have
// theirs beside the reports they check.
func TestProbePartsLight(t *testing.T) {
	t.Parallel()
	noTests := withoutFile("calc/calc_test.go")
	cases := []struct {
		name     string
		rules    []string
		edits    []fixtureEdit
		id, kind string
		want     string
		findings []string
	}{
		{"V-01 execution cannot decide without go", ids("V-01"), nil,
			"V-01", "execution", "undecided", []string{"`go` is not installed on this machine"}},
		{"V-01 probe fails for a component with no tests", ids("V-01"), []fixtureEdit{noTests},
			"V-01", "probe", "fail", []string{"component . has no tests"}},
		{"V-02 probe cannot decide with nowhere to plant", ids("V-02"), []fixtureEdit{noTests},
			"V-02", "probe", "undecided", nil},
		{"V-03 execution cannot decide without go", ids("V-03"), nil,
			"V-03", "execution", "undecided", []string{"`go` is not installed on this machine"}},
		{"A-03 probe fails with no PreToolUse hook", ids("A-03"), nil,
			"A-03", "probe", "fail", []string{
				"with PATH reduced to the operating system's directories, ",
				"with .claude/hooks/policy.json replaced by `{`, ",
				"deny cases were not refused",
			}},
		{"A-05 probe fails with no Stop hook", ids("A-05"), nil,
			"A-05", "probe", "fail", []string{"has no Stop hook"}},
		{"A-07 execution passes with no SessionStart hook", ids("A-07"),
			[]fixtureEdit{withFile(".claude/settings.json", "{}\n")},
			"A-07", "execution", "pass", nil},
		{"R-01 execution fails when A-02's deny cases are not refused", ids("R-01", "A-02"), nil,
			"R-01", "execution", "fail", []string{"no PreToolUse hook matches Bash"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rep, _ := auditJSONRules(t, registryRules(t, c.rules...), probeFixture(t, c.edits...))
			assertPartJSON(t, rep, c.id, c.kind, c.want, c.findings...)
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
	dir := probeFixture(t, sessionStartHooks("startup", "exit 0"))
	env := append(probeEnvWithPath(t, os.Getenv("PATH")),
		"CLAUDE_CODE_GIT_BASH_PATH="+filepath.Join(t.TempDir(), "missing", "bash.exe"))
	r := runVibeAuditSeams(t, contractSeams{rules: registryRules(t, "A-07"), probeEnv: env}, "--repo-root", dir, "--json")
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("no JSON report: %v\n%s", err, r.stderr)
	}
	assertPartJSON(t, rep, "A-07", "execution", "pass")
}
