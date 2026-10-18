package probetest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// The execution parts (spec 0053). The Go probe fixture needs only Go:
// its entrypoints and hooks run go run ./ci <step>.

var inherit = contract.ProbeOptions{}

// A-07, execution part: every SessionStart hook that matches startup
// exits 0 in the reset copy and changes no file.
func TestRuleA07Execution(t *testing.T) {
	t.Parallel()
	t.Run("a hook that prints facts passes", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t), ids("A-07"), inherit)
		assertExecution(t, rep, "A-07", pass)
	})
	t.Run("a hook that writes a file fails and names it", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, sessionStartHooks("startup", goHook("session-start-write"))), ids("A-07"), inherit)
		assertExecution(t, rep, "A-07", fail, ".claude/settings.json", "session.log")
	})
	t.Run("a hook that exits non-zero fails and shows its output", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, sessionStartHooks("startup", goHook("fail"))), ids("A-07"), inherit)
		assertExecution(t, rep, "A-07", fail, "exited 1", "failing on purpose")
	})
	t.Run("a hook whose matcher does not match startup does not run", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, sessionStartHooks("resume", goHook("fail"))), ids("A-07"), inherit)
		assertExecution(t, rep, "A-07", pass)
	})
	t.Run("no bash cannot decide", func(t *testing.T) {
		t.Parallel()
		dir := probeFixture(t)
		goPath, err := exec.LookPath("go")
		if err != nil {
			t.Skip("go is not on PATH")
		}
		goDir := filepath.Dir(goPath)
		for _, name := range []string{"bash", "bash.exe", "git", "git.exe"} {
			if _, err := os.Stat(filepath.Join(goDir, name)); err == nil {
				t.Skipf("%s sits beside go in %s, so PATH cannot hold go without bash", name, goDir)
			}
		}
		var env []string
		for _, kv := range os.Environ() {
			name, _, _ := strings.Cut(kv, "=")
			if strings.EqualFold(name, "PATH") || strings.EqualFold(name, "CLAUDE_CODE_GIT_BASH_PATH") {
				continue
			}
			env = append(env, kv)
		}
		env = append(env, "PATH="+goDir)
		rep := audit(t, dir, ids("A-07"), contract.ProbeOptions{Env: env})
		assertExecution(t, rep, "A-07", undecided, "bash")
	})
}

// V-03: with the format defect planted, fmt-check leaves only the planted
// file in git status, unchanged.
func TestRuleV03(t *testing.T) {
	t.Parallel()
	t.Run("a fmt-check that only lists passes", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t), ids("V-03"), inherit)
		assertExecution(t, rep, "V-03", pass)
	})
	t.Run("a fmt-check that rewrites fails and names the file", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints("  fmt-check: go run ./ci fmt\n")), ids("V-03"), inherit)
		assertExecution(t, rep, "V-03", fail, "vibe.yaml", "calc/vibe_probe_format_test.go", "was changed")
	})
	t.Run("nowhere to plant cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withoutFile("calc/calc_test.go")), ids("V-03"), inherit)
		assertExecution(t, rep, "V-03", undecided, "component . has no tests")
	})
	t.Run("a word with an expansion cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints("  fmt-check: go run ./ci $STEP\n")), ids("V-03"), inherit)
		assertExecution(t, rep, "V-03", undecided, "without a shell", "parameter expansion")
	})
	t.Run("a tool that is not installed cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints("  fmt-check: vibe-no-such-tool check\n")), ids("V-03"), inherit)
		assertExecution(t, rep, "V-03", undecided, "`vibe-no-such-tool` is not installed on this machine")
	})
}

// D-01: each component's lockfile matches its manifest, with the tools
// installed here. The cases that need no tool are in internal/cli.
func TestRuleD01(t *testing.T) {
	t.Parallel()
	t.Run("go mod tidy changes nothing", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t), ids("D-01"), inherit)
		assertExecution(t, rep, "D-01", pass)
	})
	t.Run("an unused require fails", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t,
			withFile("go.mod", "module example.com/probe\n\ngo 1.27.0\n\nrequire (\n\texample.com/lib v0.0.0\n\texample.com/unused v0.0.0\n)\n\n"+
				"replace example.com/lib => ./lib\n\nreplace example.com/unused => ./unused\n"),
			withFile("unused/go.mod", "module example.com/unused\n\ngo 1.27.0\n"),
			withFile("unused/unused.go", "// Package unused is required and never imported.\npackage unused\n"),
		), ids("D-01"), inherit)
		assertExecution(t, rep, "D-01", fail, "go.sum: does not match its manifest", "git diff --exit-code")
	})
	t.Run("dependencies without a lockfile fail", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withoutFile("go.sum")), ids("D-01"), inherit)
		assertExecution(t, rep, "D-01", fail, "go.mod: component . has dependencies and no recognized lockfile")
	})
}

// R-01, execution part: with vibe replaced by the shim, verify passes;
// every call of vibe is listed.
func TestRuleR01Execution(t *testing.T) {
	t.Parallel()
	// Without Claude Code there is no A-02 half.
	noAgents := section{"agents", "agents: []\n"}
	t.Run("verify without vibe passes", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, probeConfig(noAgents)), ids("R-01"), inherit)
		assertExecution(t, rep, "R-01", pass)
	})
	t.Run("verify that calls vibe fails and lists the call", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, probeConfig(noAgents,
			section{"entrypoints", probeEntrypoints("  verify: [go run ./ci fmt-check, go run ./ci call-vibe]\n")})), ids("R-01"), inherit)
		assertExecution(t, rep, "R-01", fail, "vibe not available", `verify ran "vibe version"`)
	})
	t.Run("a hook that calls vibe is listed", func(t *testing.T) {
		t.Parallel()
		// A-07 runs the hook; R-01 also reads A-02's verdict.
		rep := audit(t, probeFixture(t, sessionStartHooks("startup", "vibe audit --static")), ids("A-02", "A-07", "R-01"), inherit)
		assertExecution(t, rep, "A-07", fail, "exited 127", "vibe is not available during the R-01 probe")
		// The fixture has no guard, so R-01's A-02 half fails as well.
		assertExecution(t, rep, "R-01", fail, `the SessionStart hook ran "vibe audit --static"`)
	})
	t.Run("with Claude Code and no guard the A-02 deny cases fail R-01", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t), ids("A-02", "R-01"), inherit)
		assertExecution(t, rep, "R-01", fail, "no PreToolUse hook matches Bash, so its")
	})
	t.Run("a verify that runs too long cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, probeConfig(noAgents,
			section{"entrypoints", probeEntrypoints("  verify: go run ./ci sleep\n")})), ids("R-01"),
			contract.ProbeOptions{Timeout: 3 * time.Second})
		assertExecution(t, rep, "R-01", undecided, "timed out after 3s")
	})
}

// setup runs once, first; when it fails, no execution or probe part can
// be decided.
func TestProbeSetup(t *testing.T) {
	t.Parallel()
	t.Run("a failing setup leaves every part undecided", func(t *testing.T) {
		t.Parallel()
		parts := ids("V-03", "A-07", "D-01", "R-01")
		rep := audit(t, probeFixture(t, withEntrypoints("  setup: go run ./ci fail\n")), parts, inherit)
		for _, id := range parts {
			if r := rule(t, rep, id); r.Result != undecided {
				t.Errorf("%s = %s, want undecided", id, r.Result)
			}
		}
		assertExecution(t, rep, "V-03", undecided, "setup did not succeed", "failing on purpose")
	})
	t.Run("a setup that succeeds lets the parts run", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints("  setup: go run ./ci ok\n")), ids("V-03"), inherit)
		assertExecution(t, rep, "V-03", pass)
	})
}
