package probetest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// The planted-defect probes (spec 0053, #104): V-01, V-02 and A-05's
// probe part, with real processes.

// missed is how V-01 and V-02 name the defect kinds that did not make an
// entrypoint fail: the list ends the message.
func missed(kinds string) string { return "did not make it fail: " + kinds + " Fix:" }

// refuteMissed checks that V-01's findings do not list any of kinds
// among the defect kinds that did not make the entrypoint fail.
func refuteMissed(t *testing.T, rep *contract.Report, kinds ...string) {
	const id = "V-01"
	t.Helper()
	text := findingText(rule(t, rep, id))
	for _, k := range kinds {
		for _, s := range []string{"fail: " + k, ", " + k + " Fix:", ", " + k + ","} {
			if strings.Contains(text, s) {
				t.Errorf("%s findings name %q as missed\nfindings:\n%s", id, k, text)
			}
		}
	}
}

// The Python and TypeScript probe fixtures' setup installs the tools, so
// the fixtures ignore what it writes.
var (
	pythonIgnores = withFile(".gitignore", ".venv/\n__pycache__/\n.ruff_cache/\n.pytest_cache/\n")
	nodeIgnores   = withFile(".gitignore", "node_modules/\n")
)

// Every defect sample of a language is caught by its usual tools, and a
// verify without its test step names "test".
func TestRuleV01OtherLanguages(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	cases := []struct {
		language, tool, fixture string
		ignores                 fixtureEdit
		withoutTests            string
		planted                 string
	}{
		{"python", "uv", probePythonFixture, pythonIgnores,
			"  verify: [uv run --locked ruff format --check ., uv run --locked ruff check .]\n", "vibe_probe_test.py"},
		{"typescript", "pnpm", probeTypeScriptFixture, nodeIgnores,
			"  verify: [pnpm exec prettier --check \"**/*.ts\", pnpm exec tsc]\n", "vibe_probe.test.ts"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			t.Parallel()
			requireTool(t, tc.tool)
			t.Run("every defect fails verify and verify-fast", func(t *testing.T) {
				t.Parallel()
				rep := audit(t, fixtureFrom(t, tc.fixture, tc.ignores), ids("V-01", "V-02", "V-03"), inherit)
				assertPart(t, rep, "V-01", contract.Execution, pass)
				assertPart(t, rep, "V-01", contract.Probe, pass)
				assertPart(t, rep, "V-02", contract.Probe, pass)
				assertPart(t, rep, "V-03", contract.Execution, pass)
			})
			t.Run("verify without the test step names test", func(t *testing.T) {
				t.Parallel()
				cfg, err := os.ReadFile(filepath.Join(tc.fixture, "vibe.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				var lines []string
				for _, l := range strings.SplitAfter(string(cfg), "\n") {
					if strings.HasPrefix(l, "  verify:") {
						l = tc.withoutTests
					}
					lines = append(lines, l)
				}
				rep := audit(t, fixtureFrom(t, tc.fixture, tc.ignores,
					withFile("vibe.yaml", strings.Join(lines, ""))), ids("V-01"), inherit)
				assertPart(t, rep, "V-01", contract.Probe, fail, missed("test"), tc.planted)
				refuteMissed(t, rep, "format", "static analysis")
			})
		})
	}
}

// The passing and the no-tests cases of V-01, V-02 and A-05 share one
// audit each: every audit runs verify or verify-fast several times.
func TestDefectProbes(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	t.Run("entrypoints and a Stop hook that catch every defect pass", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, stopHooks(stopHook("stop"))), ids("V-01", "V-02", "A-05"), inherit)
		assertPart(t, rep, "V-01", contract.Execution, pass)
		assertPart(t, rep, "V-01", contract.Probe, pass)
		assertPart(t, rep, "V-02", contract.Probe, pass)
		assertPart(t, rep, "A-05", contract.Probe, pass)
	})
	t.Run("a component without tests fails V-01 and leaves V-02 and A-05 undecided", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, stopHooks(stopHook("stop")), withoutFile("calc/calc_test.go")),
			ids("V-01", "V-02", "A-05"), inherit)
		assertPart(t, rep, "V-01", contract.Execution, pass)
		assertPart(t, rep, "V-01", contract.Probe, fail, "component . has no tests", "_test.go")
		assertPart(t, rep, "V-02", contract.Probe, undecided, "component . has no tests")
		assertPart(t, rep, "A-05", contract.Probe, undecided, "no component has tests")
	})
}

// The failing cases of V-01, V-02 and A-05. Each audit holds one case of
// each rule, since every audit runs verify and verify-fast several
// times; the rules read different entrypoints and hooks, so the cases do
// not mix.
func TestDefectProbeFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	t.Run("verify and verify-fast without the test step, and a Stop hook that never blocks", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, stopHooks(goHook("ok")), withEntrypoints(
			"  verify: [go run ./ci fmt-check, go run ./ci typecheck]\n"+
				"  verify-fast: [go run ./ci fmt-check, go run ./ci typecheck]\n")),
			ids("V-01", "V-02", "A-05"), inherit)
		// #104's done-when: V-01 names "test".
		assertPart(t, rep, "V-01", contract.Execution, pass)
		assertPart(t, rep, "V-01", contract.Probe, fail, "vibe.yaml", "component .", missed("test"), "calc/vibe_probe_test.go")
		refuteMissed(t, rep, "format", "static analysis")
		assertPart(t, rep, "V-02", contract.Probe, fail, "vibe.yaml", "verify-fast", missed("test"))
		assertPart(t, rep, "A-05", contract.Probe, fail, ".claude/settings.json", "no Stop hook exited 2", "calc/vibe_probe_test.go")
	})
	t.Run("verify without vet, a verify-fast that always fails, and a Stop hook that blocks a continued turn", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, stopHooks(stopHook("stop-always")), withEntrypoints(
			"  verify: [go run ./ci fmt-check, go run ./ci test]\n"+
				"  verify-fast: go run ./ci fail\n")),
			ids("V-01", "V-02", "A-05"), inherit)
		assertPart(t, rep, "V-01", contract.Probe, fail, missed("static analysis"), "calc/vibe_probe_static_analysis_test.go")
		refuteMissed(t, rep, "format", "test")
		assertPart(t, rep, "V-02", contract.Probe, undecided, "verify-fast fails without a planted defect")
		assertPart(t, rep, "A-05", contract.Probe, fail, ".claude/settings.json", "stop_hook_active true", "exited 2")
	})
	t.Run("verify without fmt-check, and no Stop hook", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t,
			withEntrypoints("  verify: [go run ./ci typecheck, go run ./ci test]\n")), ids("V-01", "A-05"), inherit)
		assertPart(t, rep, "V-01", contract.Probe, fail, missed("format"), "calc/vibe_probe_format_test.go")
		refuteMissed(t, rep, "static analysis", "test")
		assertPart(t, rep, "A-05", contract.Probe, fail, ".claude/settings.json", "has no Stop hook")
	})
	t.Run("a verify that fails without a defect leaves V-01's probe undecided", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints("  verify: go run ./ci fail\n")), ids("V-01"), inherit)
		assertPart(t, rep, "V-01", contract.Execution, fail, "exited 1", "failing on purpose")
		assertPart(t, rep, "V-01", contract.Probe, undecided, "fails without a planted defect")
	})
}
