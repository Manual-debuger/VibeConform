package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The execution and probe parts (spec 0053), through the vibe audit
// seam with real processes. The Go probe fixture needs only Go: its
// entrypoints and hooks run go run ./ci <step>.

var probeGoFixture = filepath.Join("testdata", "contract", "probe-go")

// probeFixture builds a repository from the Go probe fixture with edits.
func probeFixture(t *testing.T, edits ...fixtureEdit) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	return fixtureFrom(t, probeGoFixture, edits...)
}

// probeConfig is the probe fixture's vibe.yaml with fields replaced.
func probeConfig(changes ...section) fixtureEdit {
	base := []section{
		{"contract", "contract: 1.0.0\n"},
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

// sessionStartHooks is a .claude/settings.json with SessionStart hooks
// that run commands.
func sessionStartHooks(matcher string, commands ...string) fixtureEdit {
	var hooks []map[string]any
	for _, c := range commands {
		hooks = append(hooks, map[string]any{"type": "command", "command": c})
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{
		"SessionStart": []any{map[string]any{"matcher": matcher, "hooks": hooks}},
	}}, "", "  ")
	if err != nil {
		panic(err)
	}
	return withFile(".claude/settings.json", string(data)+"\n")
}

// goHook guards a go run hook command the way the spec's fixtures do.
func goHook(step string) string {
	return "command -v go >/dev/null 2>&1 || exit 2; go run ./ci " + step
}

// auditProbe runs the full audit with --json in this process's
// environment.
func auditProbe(t *testing.T, dir string, args ...string) jsonReport {
	t.Helper()
	r := runVibeAuditProbe(t, append([]string{"--repo-root", dir, "--json"}, args...)...)
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, r.stdout, r.stderr)
	}
	return rep
}

// assertExecution checks the result of a rule's execution part.
func assertExecution(t *testing.T, rep jsonReport, id, want string, findings ...string) {
	t.Helper()
	const kind = "execution"
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
	if got := r.Parts[i].Result; got != want {
		t.Errorf("%s %s = %s, want %s\nfindings:\n%s", id, kind, got, want, r.findingText())
	}
	text := r.findingText()
	for _, f := range findings {
		if !strings.Contains(text, f) {
			t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
		}
	}
}

// A-07, execution part: every SessionStart hook that matches startup
// exits 0 in the reset copy and changes no file.
func TestRuleA07Execution(t *testing.T) {
	t.Parallel()
	t.Run("a hook that prints facts passes", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t))
		assertExecution(t, rep, "A-07", "pass")
	})
	t.Run("a hook that writes a file fails and names it", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, sessionStartHooks("startup", goHook("session-start-write"))))
		assertExecution(t, rep, "A-07", "fail", ".claude/settings.json", "session.log")
	})
	t.Run("a hook that exits non-zero fails and shows its output", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, sessionStartHooks("startup", goHook("fail"))))
		assertExecution(t, rep, "A-07", "fail", "exited 1", "failing on purpose")
	})
	t.Run("a hook whose matcher does not match startup does not run", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, sessionStartHooks("resume", goHook("fail"))))
		assertExecution(t, rep, "A-07", "pass")
	})
	t.Run("no bash cannot decide", func(t *testing.T) {
		t.Parallel()
		dir := probeFixture(t)
		goDir := filepath.Dir(mustLookPath(t, "go"))
		for _, name := range []string{"bash", "bash.exe", "git", "git.exe"} {
			if _, err := os.Stat(filepath.Join(goDir, name)); err == nil {
				t.Skipf("%s sits beside go in %s, so PATH cannot hold go without bash", name, goDir)
			}
		}
		r := runVibeAuditSeams(t, contractSeams{probeEnv: probeEnvWithPath(t, goDir)}, "--repo-root", dir, "--json")
		var rep jsonReport
		if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
			t.Fatalf("no JSON: %v\n%s", err, r.stderr)
		}
		assertExecution(t, rep, "A-07", "undecided", "bash")
	})
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not on PATH", name)
	}
	return p
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

// tempEntries lists the entries of the operating system's temporary
// directory whose names start with prefix.
func tempEntries(t *testing.T, dir, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	return out
}

// setTempDir points the operating system's temporary directory at a new
// directory for this test.
func setTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", dir)
		t.Setenv("TEMP", dir)
	} else {
		t.Setenv("TMPDIR", dir)
	}
	return dir
}

// The audit never writes: the working tree, the index and .git are the
// same after a full audit, and the probe copy is gone (spec 0053,
// acceptance criterion 2).
func TestContractAuditNeverWrites(t *testing.T) {
	dir := probeFixture(t,
		// An untracked and an ignored file stay out of the copy and stay
		// where they are.
		withFile(".gitignore", "build/\n"),
		withUntracked("notes.txt", "mine\n"),
		withUntracked("build/out.bin", "artifact\n"),
		sessionStartHooks("startup", goHook("session-start-write")),
	)
	tmp := setTempDir(t)
	before := treeHash(t, dir)
	auditProbe(t, dir)
	after := treeHash(t, dir)
	if len(before) != len(after) {
		t.Errorf("file count changed: %d before, %d after", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Errorf("%s changed", p)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s appeared", p)
		}
	}
	if left := tempEntries(t, tmp, "vibe-audit-"); len(left) > 0 {
		t.Errorf("the probe copy was not deleted: %v", left)
	}
}

func TestContractAuditKeepProbeCopy(t *testing.T) {
	dir := probeFixture(t)
	tmp := setTempDir(t)
	r := runVibeAuditProbe(t, "--repo-root", dir, "--keep-probe-copy")
	assertStderr(t, r, "vibe audit: keeping the probe copy at ")
	left := tempEntries(t, tmp, "vibe-audit-")
	if len(left) != 1 {
		t.Fatalf("temporary directory holds %v, want one kept probe copy", left)
	}
	if _, err := os.Stat(filepath.Join(tmp, left[0], "copy", "calc", "calc_test.go")); err != nil {
		t.Errorf("kept copy lacks a tracked file: %v", err)
	}
}

func TestContractAuditProgress(t *testing.T) {
	dir := probeFixture(t)
	r := runVibeAuditProbe(t, "--repo-root", dir)
	assertStderr(t, r, "vibe audit: the SessionStart hook")
	rj := runVibeAuditProbe(t, "--repo-root", dir, "--json")
	if strings.Contains(rj.stderr, "vibe audit: the SessionStart hook") {
		t.Errorf("--json printed progress lines:\n%s", rj.stderr)
	}
}

// V-03: with the format defect planted, fmt-check leaves only the planted
// file in git status, unchanged.
func TestRuleV03(t *testing.T) {
	t.Parallel()
	t.Run("a fmt-check that only lists passes", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t))
		assertExecution(t, rep, "V-03", "pass")
	})
	t.Run("a fmt-check that rewrites fails and names the file", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, withEntrypoints("  fmt-check: go run ./ci fmt\n")))
		assertExecution(t, rep, "V-03", "fail", "vibe.yaml", "calc/vibe_probe_format_test.go", "was changed")
	})
	t.Run("nowhere to plant cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, withoutFile("calc/calc_test.go")))
		assertExecution(t, rep, "V-03", "undecided", "component . has no tests")
	})
	t.Run("a word with an expansion cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, withEntrypoints("  fmt-check: go run ./ci $STEP\n")))
		assertExecution(t, rep, "V-03", "undecided", "without a shell", "parameter expansion")
	})
	t.Run("a tool that is not installed cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, withEntrypoints("  fmt-check: vibe-no-such-tool check\n")))
		assertExecution(t, rep, "V-03", "undecided", "`vibe-no-such-tool` is not installed on this machine")
	})
}

// D-01: each component's lockfile matches its manifest.
func TestRuleD01(t *testing.T) {
	t.Parallel()
	t.Run("go mod tidy changes nothing", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t))
		assertExecution(t, rep, "D-01", "pass")
	})
	t.Run("an unused require fails", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t,
			withFile("go.mod", "module example.com/probe\n\ngo 1.27.0\n\nrequire (\n\texample.com/lib v0.0.0\n\texample.com/unused v0.0.0\n)\n\n"+
				"replace example.com/lib => ./lib\n\nreplace example.com/unused => ./unused\n"),
			withFile("unused/go.mod", "module example.com/unused\n\ngo 1.27.0\n"),
			withFile("unused/unused.go", "// Package unused is required and never imported.\npackage unused\n"),
		))
		assertExecution(t, rep, "D-01", "fail", "go.sum: does not match its manifest", "git diff --exit-code")
	})
	t.Run("dependencies without a lockfile fail", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, withoutFile("go.sum")))
		assertExecution(t, rep, "D-01", "fail", "go.mod: component . has dependencies and no recognized lockfile")
	})
	t.Run("no dependencies and no lockfile pass", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditJSON(t, contractFixture(t))
		assertExecution(t, rep, "D-01", "pass")
	})
	t.Run("a Python component with dependencies and no lockfile fails", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditJSON(t, contractFixture(t, withoutFile("go.mod"),
			withFile("pyproject.toml", "[project]\nname = \"svc\"\ndependencies = [\n  \"requests>=2\",  # http\n]\n")))
		assertExecution(t, rep, "D-01", "fail", "pyproject.toml: component . has dependencies", "uv.lock or poetry.lock")
	})
	t.Run("a Python component with only python in poetry passes", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditJSON(t, contractFixture(t, withoutFile("go.mod"),
			withFile("pyproject.toml", "[project]\nname = \"svc\"\ndependencies = []\n\n[tool.poetry.dependencies]\npython = \"^3.12\"\n")))
		assertExecution(t, rep, "D-01", "pass")
	})
	t.Run("a Python component with a poetry group fails", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditJSON(t, contractFixture(t, withoutFile("go.mod"),
			withFile("pyproject.toml", "[project]\nname = 'svc'\n\n[tool.poetry.group.dev.dependencies]\npytest = { version = \"^8\" }\n")))
		assertExecution(t, rep, "D-01", "fail", "has dependencies")
	})
	t.Run("a TypeScript component with devDependencies and no lockfile fails", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditJSON(t, contractFixture(t, withoutFile("go.mod"),
			withFile("package.json", `{"name": "web", "devDependencies": {"vitest": "^2"}}`+"\n")))
		assertExecution(t, rep, "D-01", "fail", "package.json: component . has dependencies", "pnpm-lock.yaml or package-lock.json")
	})
	t.Run("a lockfile check whose tool is not installed cannot decide", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditJSON(t, contractFixture(t, withoutFile("go.mod"),
			withFile("package.json", `{"name": "web"}`+"\n"), withFile("pnpm-lock.yaml", "lockfileVersion: '9.0'\n")))
		assertExecution(t, rep, "D-01", "undecided", "`pnpm` is not installed on this machine")
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
		rep := auditProbe(t, probeFixture(t, probeConfig(noAgents)))
		assertExecution(t, rep, "R-01", "pass")
	})
	t.Run("verify that calls vibe fails and lists the call", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, probeConfig(noAgents,
			section{"entrypoints", probeEntrypoints("  verify: [go run ./ci fmt-check, go run ./ci call-vibe]\n")})))
		assertExecution(t, rep, "R-01", "fail", "vibe not available", `verify ran "vibe version"`)
	})
	t.Run("a hook that calls vibe is listed", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, sessionStartHooks("startup", "vibe audit --static")))
		assertExecution(t, rep, "A-07", "fail", "exited 127", "vibe is not available during the R-01 probe")
		// The fixture has no guard, so R-01's A-02 half fails as well.
		assertExecution(t, rep, "R-01", "fail", `the SessionStart hook ran "vibe audit --static"`)
	})
	t.Run("with Claude Code and no guard the A-02 deny cases fail R-01", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t))
		assertExecution(t, rep, "R-01", "fail", "no PreToolUse hook matches Bash, so its")
	})
	t.Run("a verify that runs too long cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, probeConfig(noAgents,
			section{"entrypoints", probeEntrypoints("  verify: go run ./ci sleep\n")})), "--probe-timeout", "3s")
		assertExecution(t, rep, "R-01", "undecided", "timed out after 3s")
	})
}

// setup runs once, first; when it fails, no execution or probe part can
// be decided.
func TestProbeSetup(t *testing.T) {
	t.Parallel()
	t.Run("a failing setup leaves every part undecided", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, withEntrypoints("  setup: go run ./ci fail\n")))
		for _, id := range []string{"V-03", "A-07", "D-01", "R-01"} {
			if r := rep.rule(t, id); r.Result != "undecided" {
				t.Errorf("%s = %s, want undecided", id, r.Result)
			}
		}
		assertExecution(t, rep, "V-03", "undecided", "setup did not succeed", "failing on purpose")
	})
	t.Run("a setup that succeeds lets the parts run", func(t *testing.T) {
		t.Parallel()
		rep := auditProbe(t, probeFixture(t, withEntrypoints("  setup: go run ./ci ok\n")))
		assertExecution(t, rep, "V-03", "pass")
	})
}

func TestContractAuditProbeTimeoutMustBePositive(t *testing.T) {
	r := runVibeAudit(t, "--repo-root", contractFixture(t), "--probe-timeout", "0s")
	assertExit(t, r, 1)
	assertStderr(t, r, "--probe-timeout must be a positive duration")
}

func TestContractAuditProbeFlagsNeedV2(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDefaults)
	for _, flag := range [][]string{{"--probe-timeout", "1m"}, {"--keep-probe-copy"}} {
		r := runVibeAudit(t, append([]string{"--repo-root", dir}, flag...)...)
		assertExit(t, r, 1)
		assertStderr(t, r, flag[0], "needs a version 2 configuration")
	}
}
