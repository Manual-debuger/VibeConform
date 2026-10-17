package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// The planted-defect probes (spec 0053, #104): V-01, V-02 and A-05's
// probe part, through the vibe audit seam with real processes.

// auditProbeRules runs the full audit with --json in this process's
// environment, with only the named rules of the registry, so that a test
// waits only for the runs it reads.
func auditProbeRules(t *testing.T, dir string, ids ...string) jsonReport {
	t.Helper()
	var rules []contract.Rule
	for _, r := range contract.Rules() {
		if slices.Contains(ids, r.ID) {
			rules = append(rules, r)
		}
	}
	r := runVibeAuditSeams(t, contractSeams{rules: rules}, "--repo-root", dir, "--json")
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, r.stdout, r.stderr)
	}
	return rep
}

// assertPart checks the result of one part of a rule and that the rule's
// findings hold each of findings.
func assertPart(t *testing.T, rep jsonReport, id, kind, want string, findings ...string) {
	t.Helper()
	r := rep.rule(t, id)
	i := slices.IndexFunc(r.Parts, func(p struct {
		Check  string `json:"check"`
		Result string `json:"result"`
	}) bool {
		return p.Check == kind
	})
	if i < 0 {
		t.Fatalf("%s has no %s part: %+v", id, kind, r.Parts)
	}
	text := r.findingText()
	if got := r.Parts[i].Result; got != want {
		t.Errorf("%s %s = %s, want %s\nfindings:\n%s", id, kind, got, want, text)
	}
	for _, f := range findings {
		if !strings.Contains(text, f) {
			t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
		}
	}
}

// missed is how V-01 and V-02 name the defect kinds that did not make an
// entrypoint fail: the list ends the message.
func missed(kinds string) string { return "did not make it fail: " + kinds + " Fix:" }

// refuteMissed checks that V-01's findings do not list any of kinds
// among the defect kinds that did not make the entrypoint fail.
func refuteMissed(t *testing.T, rep jsonReport, kinds ...string) {
	const id = "V-01"
	t.Helper()
	text := rep.rule(t, id).findingText()
	for _, k := range kinds {
		for _, s := range []string{"fail: " + k, ", " + k + " Fix:", ", " + k + ","} {
			if strings.Contains(text, s) {
				t.Errorf("%s findings name %q as missed\nfindings:\n%s", id, k, text)
			}
		}
	}
}

// stopHooks is a .claude/settings.json with one Stop entry that runs
// commands.
func stopHooks(commands ...string) fixtureEdit {
	var hooks []map[string]any
	for _, c := range commands {
		hooks = append(hooks, map[string]any{"type": "command", "command": c})
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{
		"Stop": []any{map[string]any{"hooks": hooks}},
	}}, "", "  ")
	if err != nil {
		panic(err)
	}
	return withFile(".claude/settings.json", string(data)+"\n")
}

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

// The Python and TypeScript probe fixtures: one root component each, with
// the language's usual tools. Their setup installs the tools, so the
// fixtures ignore what it writes.
var (
	probePythonFixture     = filepath.Join("testdata", "contract", "probe-python")
	probeTypeScriptFixture = filepath.Join("testdata", "contract", "probe-typescript")
	pythonIgnores          = withFile(".gitignore", ".venv/\n__pycache__/\n.ruff_cache/\n.pytest_cache/\n")
	nodeIgnores            = withFile(".gitignore", "node_modules/\n")
)

// Every defect sample of a language is caught by its usual tools, and a
// verify without its test step names "test".
func TestRuleV01OtherLanguages(t *testing.T) {
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
				rep := auditProbeRules(t, fixtureFrom(t, tc.fixture, tc.ignores), "V-01", "V-02", "V-03")
				assertPart(t, rep, "V-01", "execution", "pass")
				assertPart(t, rep, "V-01", "probe", "pass")
				assertPart(t, rep, "V-02", "probe", "pass")
				assertPart(t, rep, "V-03", "execution", "pass")
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
				rep := auditProbeRules(t, fixtureFrom(t, tc.fixture, tc.ignores,
					withFile("vibe.yaml", strings.Join(lines, ""))), "V-01")
				assertPart(t, rep, "V-01", "probe", "fail", missed("test"), tc.planted)
				refuteMissed(t, rep, "format", "static analysis")
			})
		})
	}
}

// stopHook is a guarded go run Stop hook. go run exits 1 whatever the
// program's exit code, so the command turns a failure into exit 2.
func stopHook(step string) string { return goHook(step) + " || exit 2" }

// V-01: verify passes in the probe copy and fails with each planted
// defect in each component.
func TestRuleV01(t *testing.T) {
	t.Parallel()
	t.Run("a verify that catches every defect passes", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t), "V-01")
		assertPart(t, rep, "V-01", "execution", "pass")
		assertPart(t, rep, "V-01", "probe", "pass")
	})
	t.Run("verify without the test step fails and names test", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t,
			withEntrypoints("  verify: [go run ./ci fmt-check, go run ./ci typecheck]\n")), "V-01")
		assertPart(t, rep, "V-01", "execution", "pass")
		assertPart(t, rep, "V-01", "probe", "fail", "vibe.yaml", "component .", missed("test"), "calc/vibe_probe_test.go")
		refuteMissed(t, rep, "format", "static analysis")
	})
	t.Run("verify without vet fails and names static analysis", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t,
			withEntrypoints("  verify: [go run ./ci fmt-check, go run ./ci test]\n")), "V-01")
		assertPart(t, rep, "V-01", "probe", "fail", missed("static analysis"), "calc/vibe_probe_static_analysis_test.go")
		refuteMissed(t, rep, "format", "test")
	})
	t.Run("verify without fmt-check fails and names format", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t,
			withEntrypoints("  verify: [go run ./ci typecheck, go run ./ci test]\n")), "V-01")
		assertPart(t, rep, "V-01", "probe", "fail", missed("format"), "calc/vibe_probe_format_test.go")
		refuteMissed(t, rep, "static analysis", "test")
	})
	t.Run("a component without tests fails", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, withoutFile("calc/calc_test.go")), "V-01")
		assertPart(t, rep, "V-01", "execution", "pass")
		assertPart(t, rep, "V-01", "probe", "fail", "component . has no tests", "_test.go")
	})
	t.Run("a verify that fails without a defect leaves the probe undecided", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, withEntrypoints("  verify: go run ./ci fail\n")), "V-01")
		assertPart(t, rep, "V-01", "execution", "fail", "exited 1", "failing on purpose")
		assertPart(t, rep, "V-01", "probe", "undecided", "fails without a planted defect")
	})
}

// V-02: verify-fast fails with each planted defect in each component.
func TestRuleV02(t *testing.T) {
	t.Parallel()
	t.Run("a verify-fast that catches every defect passes", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t), "V-02")
		assertPart(t, rep, "V-02", "probe", "pass")
	})
	t.Run("a verify-fast that misses a defect fails and names it", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t,
			withEntrypoints("  verify-fast: [go run ./ci fmt-check, go run ./ci typecheck]\n")), "V-02")
		assertPart(t, rep, "V-02", "probe", "fail", "vibe.yaml", "verify-fast", missed("test"))
	})
	t.Run("a verify-fast that always fails cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, withEntrypoints("  verify-fast: go run ./ci fail\n")), "V-02")
		assertPart(t, rep, "V-02", "probe", "undecided", "verify-fast fails without a planted defect")
	})
	t.Run("a component without tests cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, withoutFile("calc/calc_test.go")), "V-02")
		assertPart(t, rep, "V-02", "probe", "undecided", "component . has no tests")
	})
}

// A-05, probe part: with a failing test planted, a Stop hook blocks the
// turn, and lets a continued turn end.
func TestRuleA05Probe(t *testing.T) {
	t.Parallel()
	t.Run("a Stop hook that blocks once passes", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, stopHooks(stopHook("stop"))), "A-05")
		assertPart(t, rep, "A-05", "probe", "pass")
	})
	t.Run("a Stop hook that never blocks fails", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, stopHooks(goHook("ok"))), "A-05")
		assertPart(t, rep, "A-05", "probe", "fail", ".claude/settings.json", "no Stop hook exited 2", "calc/vibe_probe_test.go")
	})
	t.Run("a Stop hook that blocks a continued turn fails", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, stopHooks(stopHook("stop-always"))), "A-05")
		assertPart(t, rep, "A-05", "probe", "fail", ".claude/settings.json", "stop_hook_active true", "exited 2")
	})
	t.Run("no Stop hook fails", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t), "A-05")
		assertPart(t, rep, "A-05", "probe", "fail", ".claude/settings.json", "has no Stop hook")
	})
	t.Run("no component with tests cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditProbeRules(t, probeFixture(t, stopHooks(stopHook("stop")), withoutFile("calc/calc_test.go")), "A-05")
		assertPart(t, rep, "A-05", "probe", "undecided", "no component has tests")
	})
}
