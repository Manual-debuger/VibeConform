package contract_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// The guard probes A-02 and A-03 (spec 0053, "Guard probe fixtures") with
// real guards. Each fixture runs hundreds of hook processes, so these
// tests live in this package, with a test-time budget of their own, and
// call the audit engine, contract.Audit, which vibe audit runs; the
// rendering of what they find (findings, guard_exception_uses, the text
// report) is tested through the vibe audit command in internal/cli.
// Only the rules a test needs run.

func TestMain(m *testing.M) {
	// The R-01 shim is a copy of the running executable: this test binary.
	if code, ok := contract.RunAsShim(); ok {
		os.Exit(code)
	}
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		_ = os.Unsetenv(v)
	}
	code := m.Run()
	if dir, err := vibeGuardDir(); err == nil && dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

// probeGoFixture is the Go probe fixture of the vibe audit tests.
var probeGoFixture = filepath.Join("..", "cli", "testdata", "contract", "probe-go")

// guardFixture copies the probe fixture into a new Git repository, with
// files (slash-separated path to content) added, and returns its root.
// testdata keeps .claude under dot-claude, which the copy renames.
func guardFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	dir := t.TempDir()
	err := filepath.WalkDir(probeGoFixture, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(probeGoFixture, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, strings.ReplaceAll(rel, "dot-claude", ".claude"))
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
		t.Fatal(err)
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"-c", "core.autocrlf=false", "add", "-A"}} {
		cmd := exec.Command("git", args...) // #nosec G204 -- fixed arguments
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// guardSettings is a .claude/settings.json whose PreToolUse hooks, one
// entry with the reference matcher, run commands.
func guardSettings(commands ...string) string {
	var hooks []map[string]any
	for _, c := range commands {
		hooks = append(hooks, map[string]any{"type": "command", "command": c})
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{
		"PreToolUse": []any{map[string]any{"matcher": guard.ReferencePreToolUseMatcher, "hooks": hooks}},
	}}, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(data) + "\n"
}

// guardStopSettings is a .claude/settings.json with the reference
// PreToolUse entry running pre and one Stop entry running stops.
func guardStopSettings(pre string, stops ...string) string {
	var stopHooks []map[string]any
	for _, c := range stops {
		stopHooks = append(stopHooks, map[string]any{"type": "command", "command": c})
	}
	data, err := json.MarshalIndent(map[string]any{"hooks": map[string]any{
		"PreToolUse": []any{map[string]any{"matcher": guard.ReferencePreToolUseMatcher,
			"hooks": []any{map[string]any{"type": "command", "command": pre}}}},
		"Stop": []any{map[string]any{"hooks": stopHooks}},
	}}, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(data) + "\n"
}

var referenceGuard = guard.ReferencePreToolUse("v0.0.0-test")

// brokenGuard is a fixture guard that lets rm -r -f build through and
// hands every other call to vibe-guard.
const brokenGuard = `input=$(cat); case "$input" in *'"rm -r -f build"'*) exit 0;; esac; ` +
	`printf '%s' "$input" | vibe-guard hook`

// vibeGuardDir holds the vibe-guard this test binary built, once.
var vibeGuardDir = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "vibe-guard-bin-")
	if err != nil {
		return "", err
	}
	exe := "vibe-guard"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, exe), "github.com/Manual-debuger/VibeConform/cmd/vibe-guard")
	if out, err := cmd.CombinedOutput(); err != nil {
		return dir, fmt.Errorf("building vibe-guard: %w\n%s", err, out)
	}
	return dir, nil
})

func builtVibeGuard(t *testing.T) string {
	t.Helper()
	dir, err := vibeGuardDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// requireBash skips without the bash hooks run with, unless
// VIBE_TEST_REQUIRE_TOOLS names bash.
func requireBash(t *testing.T) {
	t.Helper()
	name := "bash"
	if runtime.GOOS == "windows" {
		name = "git" // Git Bash is found next to git.exe
	}
	if _, err := exec.LookPath(name); err != nil {
		if slices.Contains(strings.Split(os.Getenv("VIBE_TEST_REQUIRE_TOOLS"), ","), "bash") {
			t.Fatalf("%s is not on PATH and VIBE_TEST_REQUIRE_TOOLS requires bash", name)
		}
		t.Skipf("%s is not on PATH", name)
	}
}

// audit runs the rules ids against the repository at dir, with extra
// directories first on the probe runs' PATH.
func audit(t *testing.T, dir string, ids []string, pathFirst ...string) *contract.Report {
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
	env := os.Environ()
	if len(pathFirst) > 0 {
		path := strings.Join(append(slices.Clone(pathFirst), os.Getenv("PATH")), string(os.PathListSeparator))
		env = slices.DeleteFunc(env, func(kv string) bool {
			name, _, _ := strings.Cut(kv, "=")
			return strings.EqualFold(name, "PATH")
		})
		env = append(env, "PATH="+path)
	}
	return contract.Audit(ctx, cfg, repo, contract.Options{Rules: rules, Probe: contract.ProbeOptions{Env: env}})
}

// part returns a rule's part of a kind.
func part(t *testing.T, rep *contract.Report, id string, kind contract.PartKind) contract.PartOutcome {
	t.Helper()
	for _, rr := range rep.Rules {
		if rr.Rule.ID != id {
			continue
		}
		for _, p := range rr.Parts {
			if p.Kind == kind {
				return p
			}
		}
	}
	t.Fatalf("the report has no %s part of %s", kind, id)
	return contract.PartOutcome{}
}

func findingText(p contract.PartOutcome) string {
	var b strings.Builder
	for _, f := range p.Findings {
		b.WriteString(f.File + ": " + f.Message + " Fix: " + f.Fix + "\n")
	}
	return b.String()
}

// assertPart checks a part's result and that its findings hold texts.
func assertPart(t *testing.T, rep *contract.Report, id string, kind contract.PartKind, want contract.Result, texts ...string) {
	t.Helper()
	p := part(t, rep, id, kind)
	if p.Result != want {
		t.Errorf("%s %s = %s, want %s\nfindings:\n%s", id, kind, p.Result, want, findingText(p))
	}
	text := findingText(p)
	for _, s := range texts {
		if !strings.Contains(text, s) {
			t.Errorf("%s findings lack %q\nfindings:\n%s", id, s, text)
		}
	}
}

// casesNamed returns the corpus cases A-02's findings name, sorted.
func casesNamed(t *testing.T, rep *contract.Report) []string {
	t.Helper()
	var out []string
	for _, f := range part(t, rep, "A-02", contract.Probe).Findings {
		rest, ok := strings.CutPrefix(f.Message, "corpus case ")
		if !ok {
			t.Errorf("A-02 finding names no corpus case: %s", f.Message)
			continue
		}
		var name string
		if err := json.NewDecoder(strings.NewReader(rest)).Decode(&name); err != nil {
			t.Errorf("A-02 finding: cannot read the case name: %s", f.Message)
			continue
		}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// A-02 and A-03 with the reference guard and variants of it.
func TestGuardProbesVibeGuard(t *testing.T) {
	t.Parallel()
	requireBash(t)
	bin := builtVibeGuard(t)
	settings := ".claude/settings.json"
	policy := ".claude/hooks/policy.json"

	t.Run("the reference guard passes A-02, A-03 and R-01's guard half", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, map[string]string{settings: guardSettings(referenceGuard)}),
			[]string{"A-02", "A-03", "R-01"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
		assertPart(t, rep, "A-03", contract.Probe, contract.ResultPass)
		assertPart(t, rep, "R-01", contract.Execution, contract.ResultPass)
	})
	t.Run("a guard that allows rm -r -f fails A-02 and R-01 and names the case", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, map[string]string{settings: guardSettings(brokenGuard)}),
			[]string{"A-02", "R-01"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail,
			`corpus case "rm -r -f (spec 0047)" (Bash "rm -r -f build") should be refused (guard rule rm-rf)`,
			"exited 0", "list an exception for rm-rf in .claude/hooks/policy.json")
		if got := casesNamed(t, rep); !slices.Equal(got, []string{"rm -r -f (spec 0047)"}) {
			t.Errorf("A-02 names %q, want only rm -r -f (spec 0047)", got)
		}
		assertPart(t, rep, "R-01", contract.Execution, contract.ResultFail, `corpus case "rm -r -f (spec 0047)"`)
	})
	t.Run("observe mode fails A-02", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, map[string]string{
			settings: guardSettings(referenceGuard),
			policy:   `{"version": 2, "mode": "observe"}` + "\n",
		}), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail, `corpus case "rm -r -f (spec 0047)"`, "exited 0")
	})
	t.Run("an exception lets its cases through and they are listed", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, map[string]string{
			settings: guardSettings(referenceGuard),
			policy: `{"version": 2, "exceptions": [` +
				`{"rule": "rm-rf", "match": "rm -r -f build", "reason": "the build directory is disposable"}]}` + "\n",
		}), []string{"A-02", "A-03"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
		// In A-03's situations the guard cannot apply the exception.
		assertPart(t, rep, "A-03", contract.Probe, contract.ResultPass)
		// The exception's words are a prefix of a simple command of each
		// of these cases, after wrappers and nested shells; vibe-guard
		// lets the same ones through, or A-02 would fail.
		var want []contract.GuardExceptionUse
		for _, c := range []string{
			"rm -r -f (spec 0047)",
			"rm with extra blanks",
			"variant (#105): quoted flags",
			"variant (#105): sudo with options, by full paths",
			"variant (#105): sh -c with separate flags",
		} {
			want = append(want, contract.GuardExceptionUse{Rule: "rm-rf", Match: "rm -r -f build", Case: c})
		}
		if !slices.Equal(rep.GuardExceptionUses, want) {
			t.Errorf("GuardExceptionUses = %+v, want %+v", rep.GuardExceptionUses, want)
		}
	})
	t.Run("a hook without the install check fails A-03", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, map[string]string{settings: guardSettings("vibe-guard hook")}),
			[]string{"A-03"}, bin)
		assertPart(t, rep, "A-03", contract.Probe, contract.ResultFail,
			"with PATH reduced to the operating system's directories, ",
			`corpus case "rm -r -f (spec 0047)"`, "exited 127", "command -v vibe-guard")
		if text := findingText(part(t, rep, "A-03", contract.Probe)); strings.Contains(text, "replaced by `{`") {
			t.Errorf("A-03 fails the invalid-policy situation too:\n%s", text)
		}
	})
	t.Run("a guard that ignores an invalid policy fails A-03", func(t *testing.T) {
		t.Parallel()
		// This fixture guard allows every call once policy.json is `{`.
		hook := `grep -q '^{$' .claude/hooks/policy.json 2>/dev/null && exit 0; ` + referenceGuard
		rep := audit(t, guardFixture(t, map[string]string{
			settings: guardSettings(hook),
			policy:   `{"version": 2}` + "\n",
		}), []string{"A-03"}, bin)
		assertPart(t, rep, "A-03", contract.Probe, contract.ResultFail,
			"with .claude/hooks/policy.json replaced by `{`, ",
			"refuse every call (exit 2) when .claude/hooks/policy.json does not parse")
	})
}

// A-08 with vibe-guard (spec 0053, "Guard probe fixtures"): the A-08
// cases in their unlock states, and a Stop hook that must remove the
// marker.
func TestGuardProbeA08(t *testing.T) {
	t.Parallel()
	requireBash(t)
	bin := builtVibeGuard(t)
	settings := ".claude/settings.json"

	t.Run("vibe-guard with a relocking Stop hook passes A-08", func(t *testing.T) {
		t.Parallel()
		dir := guardFixture(t, map[string]string{settings: guardStopSettings(referenceGuard, "vibe-guard relock")})
		rep := audit(t, dir, []string{"A-08"}, bin)
		assertPart(t, rep, "A-08", contract.Probe, contract.ResultPass)
	})
	t.Run("an exception for guardrail-settings passes A-02 and fails A-08", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, map[string]string{
			settings: guardStopSettings(referenceGuard, "vibe-guard relock"),
			".claude/hooks/policy.json": `{"version": 2, "exceptions": [` +
				`{"rule": "guardrail-settings", "match": ".claude/**", "reason": "the agent maintains its hooks"}]}` + "\n",
		}), []string{"A-02", "A-08"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
		assertPart(t, rep, "A-08", contract.Probe, contract.ResultFail,
			`corpus case "A-08 (#106): Write settings.json, no unlock" (Write ".claude/settings.json") should be refused with no unlock marker`,
			`the exception guardrail-settings ".claude/**" in .claude/hooks/policy.json does not count: guard exceptions do not satisfy A-08`)
		// Only the six edits of settings and hooks get through: the marker
		// and vibe unlock stay refused.
		if p := part(t, rep, "A-08", contract.Probe); len(p.Findings) != 6 {
			t.Errorf("A-08 has %d findings, want 6:\n%s", len(p.Findings), findingText(p))
		}
	})
	t.Run("a Stop hook without vibe-guard relock fails A-08", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, map[string]string{settings: guardStopSettings(referenceGuard, "true")}),
			[]string{"A-08"}, bin)
		assertPart(t, rep, "A-08", contract.Probe, contract.ResultFail,
			"after the Stop hooks ran with a valid unlock marker, the marker .git/vibe-unlock still exists; hook \"true\" exited 0",
			"Fix: add a Stop hook that runs `vibe-guard relock`")
		if n := len(part(t, rep, "A-08", contract.Probe).Findings); n != 1 {
			t.Errorf("A-08 has %d findings, want only the Stop hook's", n)
		}
	})
}

// The old guards of the repo-tooling modules run through the audit
// (spec 0053, "Old guards"). Each must satisfy every case #103 took over
// from their corpus; the cases #105 and #106 added that they miss are
// listed exactly. A-02 runs, and A-08, which they fail.
func TestGuardProbesOldGuards(t *testing.T) {
	t.Parallel()
	requireBash(t)
	// The cases #105 added that the old guards' regular expressions do
	// not refuse. The three guards share the regular expressions.
	missed := []string{
		"variant (#105): a quoted command name",
		"variant (#105): a backslash inside the command name",
		"variant (#105): empty quotes inside the command name",
		"variant (#105): quoted flags",
		"variant (#105): ANSI-C quoted flags",
		"variant (#105): a separator inside a quoted operand",
		"variant (#105): flags after many operands",
		"variant (#105): eval of separate words",
		"variant (#105): flags from an unquoted variable",
		"variant (#105): & with a quoted cmdlet name",
		"variant (#105): a quoted git",
		"variant (#105): git restore ./.",
		"variant (#105): git restore :(top)",
		"variant (#105): git restore of every file by a glob",
	}
	// The old guards have no guardrail-settings rule: they miss every
	// A-08 deny case #106 added.
	for _, c := range contract.GuardCorpus() {
		if c.Want == contract.WantDeny && strings.HasPrefix(c.Name, "A-08 (#106): ") {
			missed = append(missed, c.Name)
		}
	}
	slices.Sort(missed)

	// The old guards' policy as the runtime guard renders it. This
	// repository's own policy.json is vibe-guard's since it opted in
	// (policy.agent_guard, spec 0053); the examples keep the runtime guard.
	policy, err := os.ReadFile(filepath.Join("..", "..", "examples", "typescript", ".claude", "hooks", "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	modules := filepath.Join("..", "module")
	goGuard := filepath.Join(modules, "repotooling", "templates", "guard.go")
	guards := []struct {
		name    string
		tools   []string
		program string
	}{
		{"go", []string{"go"}, goGuard},
		{"python", pythonNames(), filepath.Join(modules, "pyrepotooling", "templates", "guard.py")},
		{"node", []string{"node"}, filepath.Join(modules, "tsrepotooling", "templates", "guard.mjs")},
	}
	for _, g := range guards {
		t.Run(g.name, func(t *testing.T) {
			t.Parallel()
			hook := oldGuardHook(t, g.name, g.tools, g.program)
			rep := audit(t, guardFixture(t, map[string]string{
				".claude/settings.json":     guardSettings(hook),
				".claude/hooks/policy.json": string(policy),
			}), []string{"A-02", "A-08"})
			assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail)
			assertPart(t, rep, "A-08", contract.Probe, contract.ResultFail,
				`corpus case "A-08 (#106): Write settings.json, no unlock" (Write ".claude/settings.json") should be refused`,
				"no Stop hook removes the unlock marker")
			if got := casesNamed(t, rep); !slices.Equal(got, missed) {
				t.Errorf("the %s guard misses\n%q\nwant\n%q\nfindings:\n%s", g.name, got, missed,
					findingText(part(t, rep, "A-02", contract.Probe)))
			}
		})
	}
	t.Run("a tracked policy.local.json that loosens fails A-02", func(t *testing.T) {
		t.Parallel()
		hook := oldGuardHook(t, "go", []string{"go"}, goGuard)
		rep := audit(t, guardFixture(t, map[string]string{
			".claude/settings.json":     guardSettings(hook),
			".claude/hooks/policy.json": string(policy),
			".claude/hooks/policy.local.json": `{"version": 1, "exceptions": [` +
				`{"rule": "rm-rf", "pattern": "rm -r -f build", "reason": "the build directory is disposable"}]}` + "\n",
		}), []string{"A-02"})
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail, `corpus case "rm -r -f (spec 0047)"`,
			"exceptions in the tracked .claude/hooks/policy.local.json do not count")
	})
}

// oldGuardHook is the PreToolUse command that runs an old guard with the
// fixture's policy.json, as hook:guard does: any failure refuses.
func oldGuardHook(t *testing.T, gate string, tools []string, program string) string {
	t.Helper()
	interp := requireToolOf(t, gate, tools)
	abs, err := filepath.Abs(program)
	if err != nil {
		t.Fatal(err)
	}
	if gate == "go" {
		// go run reports the guard's exit 2 as 1.
		return "command -v go >/dev/null 2>&1 || exit 2; go run " + shellQuote(abs) + " .claude/hooks/policy.json || exit 2"
	}
	return shellQuote(interp) + " " + shellQuote(abs) + " .claude/hooks/policy.json || exit 2"
}

func pythonNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"python", "python3"}
	}
	return []string{"python3", "python"}
}

// requireToolOf returns the first of names on PATH. Without one the test
// skips, unless VIBE_TEST_REQUIRE_TOOLS names gate. The Windows Store
// stubs for python do not count.
func requireToolOf(t *testing.T, gate string, names []string) string {
	t.Helper()
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil && !strings.Contains(strings.ToLower(p), "windowsapps") {
			return p
		}
	}
	if slices.Contains(strings.Split(os.Getenv("VIBE_TEST_REQUIRE_TOOLS"), ","), gate) {
		t.Fatalf("%s is not on PATH and VIBE_TEST_REQUIRE_TOOLS requires it", gate)
	}
	t.Skipf("%s is not on PATH", gate)
	return ""
}

// shellQuote quotes a path for bash, slash separated.
func shellQuote(p string) string {
	return "'" + strings.ReplaceAll(filepath.ToSlash(p), "'", `'\''`) + "'"
}
