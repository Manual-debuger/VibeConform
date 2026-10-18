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
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
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

// The probe fixtures, shared with internal/cli's tests.
var (
	cliTestdata            = filepath.Join("..", "..", "cli", "testdata", "contract")
	probeGoFixture         = filepath.Join(cliTestdata, "probe-go")
	probePythonFixture     = filepath.Join(cliTestdata, "probe-python")
	probeTypeScriptFixture = filepath.Join(cliTestdata, "probe-typescript")
)

// fixtureEdit changes a fixture before or after git add.
type fixtureEdit struct {
	path      string
	content   string
	remove    bool
	untracked bool
}

// withFile writes path (slash separated) before git add: it is tracked.
func withFile(path, content string) fixtureEdit {
	return fixtureEdit{path: path, content: content}
}

// withoutFile removes path before git add.
func withoutFile(path string) fixtureEdit {
	return fixtureEdit{path: path, remove: true}
}

// fixtureFrom copies the tree src into a temporary directory (testdata's
// dot-claude directories become .claude), applies edits and runs git
// init and git add, and returns the root.
func fixtureFrom(t *testing.T, src string, edits ...fixtureEdit) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, string(filepath.Separator))
		for i, part := range parts {
			if part == "dot-claude" {
				parts[i] = ".claude"
			}
		}
		dst := filepath.Join(dir, filepath.Join(parts...))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o750)
		}
		data, err := os.ReadFile(p) // #nosec G304 -- testdata
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o600)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", src, err)
	}
	for _, e := range edits {
		if !e.untracked {
			applyEdit(t, dir, e)
		}
	}
	git(t, dir, "init", "-q")
	git(t, dir, "-c", "core.autocrlf=false", "add", "-A")
	for _, e := range edits {
		if e.untracked {
			applyEdit(t, dir, e)
		}
	}
	return dir
}

func applyEdit(t *testing.T, dir string, e fixtureEdit) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(e.path))
	if e.remove {
		if err := os.Remove(p); err != nil {
			t.Fatalf("removing %s: %v", e.path, err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(e.content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
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
// process's environment unless probe says otherwise.
func audit(t *testing.T, dir string, ids []string, probe contract.ProbeOptions) *contract.Report {
	t.Helper()
	ctx := context.Background()
	repo, err := contract.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "vibe.yaml")) // #nosec G304 -- the fixture's
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := contract.Parse("vibe.yaml", data, repo)
	if err != nil {
		t.Fatal(err)
	}
	var rules []contract.Rule
	for _, r := range contract.Rules() {
		if slices.Contains(ids, r.ID) {
			rules = append(rules, r)
		}
	}
	if len(rules) != len(ids) {
		t.Fatalf("rules %v: some ID is not in the registry", ids)
	}
	return contract.Audit(ctx, cfg, repo, contract.Options{Rules: rules, Probe: probe})
}

// rule returns one rule's result.
func rule(t *testing.T, rep *contract.Report, id string) contract.RuleResult {
	t.Helper()
	for _, rr := range rep.Rules {
		if rr.Rule.ID == id {
			return rr
		}
	}
	t.Fatalf("the report has no rule %s", id)
	return contract.RuleResult{}
}

// findingText is every finding of a rule, one per line, as vibe audit
// --json lists them.
func findingText(rr contract.RuleResult) string {
	var b strings.Builder
	for _, f := range rr.Findings() {
		b.WriteString(f.File + ": " + f.Message + " Fix: " + f.Fix + "\n")
	}
	return b.String()
}

// assertPart checks the result of one part of a rule and that the rule's
// findings hold each of findings.
func assertPart(t *testing.T, rep *contract.Report, id string, kind contract.PartKind, want contract.Result, findings ...string) {
	t.Helper()
	rr := rule(t, rep, id)
	i := slices.IndexFunc(rr.Parts, func(p contract.PartOutcome) bool { return p.Kind == kind })
	if i < 0 {
		t.Fatalf("%s has no %s part: %+v", id, kind, rr.Parts)
	}
	text := findingText(rr)
	if got := rr.Parts[i].Result; got != want {
		t.Errorf("%s %s = %s, want %s\nfindings:\n%s", id, kind, got, want, text)
	}
	for _, f := range findings {
		if !strings.Contains(text, f) {
			t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
		}
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
