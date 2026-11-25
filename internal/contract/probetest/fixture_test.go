// Package probetest_test holds the probe fixtures of spec 0053 that run
// real tools: the execution parts (A-07, V-03, D-01, R-01, setup) and the
// planted-defect probes (V-01, V-02, A-05) with the Go, Python and
// TypeScript probe fixtures. They call contract.Audit, the engine vibe
// audit runs, with only the rules each test reads. They live in a package
// of their own so that they have go test's ten-minute budget to
// themselves: in internal/cli, beside its other tests, they pushed the
// package past it (spec 0053, "Guard probe fixtures"). What the command
// adds, the JSON and text report, is tested in internal/cli.
package probetest_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

func TestMain(m *testing.M) {
	// The R-01 shim is a copy of the running executable: this test binary.
	if code, ok := contract.RunAsShim(); ok {
		os.Exit(code)
	}
	// A git hook running the suite exports GIT_DIR; every git the tests
	// start would then use this repository instead of the fixture.
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		_ = os.Unsetenv(v)
	}
	os.Exit(m.Run())
}

// The probe fixtures, under internal/contract/testdata.
var (
	probeGoFixture         = contracttest.ProbeGo
	probePythonFixture     = contracttest.ProbePython
	probeTypeScriptFixture = contracttest.ProbeTypeScript
)

// fixtureEdit changes a fixture before or after git add.
type fixtureEdit = contracttest.Edit

// withFile writes path (slash separated) before git add: it is tracked.
func withFile(path, content string) fixtureEdit { return contracttest.WithFile(path, content) }

// withoutFile removes path before git add.
func withoutFile(path string) fixtureEdit { return contracttest.WithoutFile(path) }

// fixtureFrom copies the tree src into a temporary directory (testdata's
// dot-claude directories become .claude), applies edits and runs git
// init and git add, and returns the root: the probe copy is made with
// git.
func fixtureFrom(t *testing.T, src string, edits ...fixtureEdit) string {
	t.Helper()
	return contracttest.GitFixture(t, src, edits...)
}

// probeFixture builds a repository from the Go probe fixture with edits.
func probeFixture(t *testing.T, edits ...fixtureEdit) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	return fixtureFrom(t, probeGoFixture, edits...)
}

// section is one top-level field of vibe.yaml.
type section struct {
	key  string
	body string
}

// probeConfig is the probe fixture's vibe.yaml with fields replaced.
func probeConfig(changes ...section) fixtureEdit {
	base := []section{
		{"contract", "contract: 1.2.0\n"},
		{"platforms", "platforms: [linux, windows]\n"},
		{"agents", "agents: [claude]\n"},
		{"entrypoints", probeEntrypoints("")},
	}
	var b strings.Builder
	used := map[string]bool{}
	for _, s := range base {
		body := s.body
		for _, c := range changes {
			if c.key == s.key {
				body, used[c.key] = c.body, true
			}
		}
		b.WriteString(body)
	}
	for _, c := range changes {
		if !used[c.key] {
			b.WriteString(c.body)
		}
	}
	return withFile("vibe.yaml", b.String())
}

// probeEntrypoints is the fixture's entrypoints section with lines
// replaced or added: each line of override is "  name: command".
func probeEntrypoints(override string) string {
	lines := map[string]string{
		"verify":      "  verify: [go run ./ci fmt-check, go run ./ci typecheck, go run ./ci test]\n",
		"verify-fast": "  verify-fast: [go run ./ci fmt-check, go run ./ci typecheck, go run ./ci test]\n",
		"fmt":         "  fmt: go run ./ci fmt\n",
		"fmt-check":   "  fmt-check: go run ./ci fmt-check\n",
	}
	order := []string{"verify", "verify-fast", "fmt", "fmt-check"}
	for _, l := range strings.SplitAfter(override, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimSpace(l), ":")
		if _, ok := lines[name]; !ok {
			order = append(order, name)
		}
		lines[name] = l
	}
	var b strings.Builder
	b.WriteString("entrypoints:\n")
	for _, n := range order {
		b.WriteString(lines[n])
	}
	return b.String()
}

// withEntrypoints replaces entrypoint lines of the probe fixture.
func withEntrypoints(override string) fixtureEdit {
	return probeConfig(section{"entrypoints", probeEntrypoints(override)})
}

// hooks is a .claude/settings.json with one entry of event that runs
// commands; matcher "" means none.
func hooks(event, matcher string, commands ...string) fixtureEdit {
	var hs []map[string]any
	for _, c := range commands {
		hs = append(hs, map[string]any{"type": "command", "command": c})
	}
	entry := map[string]any{"hooks": hs}
	if matcher != "" {
		entry["matcher"] = matcher
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{event: []any{entry}}}, "", "  ")
	if err != nil {
		panic(err)
	}
	return withFile(".claude/settings.json", string(data)+"\n")
}

// sessionStartHooks is a .claude/settings.json with SessionStart hooks.
func sessionStartHooks(matcher string, commands ...string) fixtureEdit {
	return hooks("SessionStart", matcher, commands...)
}

// stopHooks is a .claude/settings.json with one Stop entry.
func stopHooks(commands ...string) fixtureEdit {
	return hooks("Stop", "", commands...)
}

// goHook guards a go run hook command the way the spec's fixtures do.
func goHook(step string) string {
	return "command -v go >/dev/null 2>&1 || exit 2; go run ./ci " + step
}

// stopHook is a guarded go run Stop hook. go run exits 1 whatever the
// program's exit code, so the command turns a failure into exit 2.
func stopHook(step string) string { return goHook(step) + " || exit 2" }

// requireTool skips the test when a tool is not on PATH, or fails it when
// VIBE_TEST_REQUIRE_TOOLS (a comma-separated list) names the tool.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err == nil {
		return
	}
	if slices.Contains(strings.Split(os.Getenv("VIBE_TEST_REQUIRE_TOOLS"), ","), name) {
		t.Fatalf("VIBE_TEST_REQUIRE_TOOLS names %s, but it is not on PATH", name)
	}
	t.Skipf("%s is not on PATH", name)
}

// ids is a short way to write a list of rule IDs.
func ids(list ...string) []string { return list }

// audit runs contract.Audit on dir with only the rules ids, in this
// process's environment unless probe says otherwise. contracttest.Audit
// checks the report for failures without a file or a fix.
func audit(t *testing.T, dir string, ids []string, probe contract.ProbeOptions) *contract.Report {
	t.Helper()
	return contracttest.Audit(t, dir, contract.Options{Rules: contracttest.Rules(t, ids...), Probe: probe})
}

// rule returns one rule's result.
func rule(t *testing.T, rep *contract.Report, id string) contract.RuleResult {
	t.Helper()
	return contracttest.RuleResult(t, rep, id)
}

// findingText is every finding of a rule, one per line, as vibe audit
// --json lists them.
func findingText(rr contract.RuleResult) string { return contracttest.FindingText(rr) }

// assertPart checks the result of one part of a rule and that the rule's
// findings hold each of findings.
func assertPart(t *testing.T, rep *contract.Report, id string, kind contract.PartKind, want contract.Result, findings ...string) {
	t.Helper()
	contracttest.AssertPart(t, rep, id, kind, want, findings...)
}

// assertListedOnce checks that exactly one finding of a rule holds text.
func assertListedOnce(t *testing.T, rep *contract.Report, id, text string) {
	t.Helper()
	rr := rule(t, rep, id)
	n := 0
	for _, f := range rr.Findings() {
		if strings.Contains(f.Message, text) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%s lists %q %d times, want once\nfindings:\n%s", id, text, n, findingText(rr))
	}
}

// assertExecution checks a rule's execution part.
func assertExecution(t *testing.T, rep *contract.Report, id string, want contract.Result, findings ...string) {
	t.Helper()
	assertPart(t, rep, id, contract.Execution, want, findings...)
}

// The results, short.
const (
	pass      = contract.ResultPass
	fail      = contract.ResultFail
	undecided = contract.ResultUndecided
)
