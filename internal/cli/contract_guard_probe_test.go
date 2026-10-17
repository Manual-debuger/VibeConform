package cli

import (
	"encoding/json"
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

// The guard probes A-02 and A-03 (spec 0053, "Guard probe fixtures"),
// through the vibe audit seam. A fixture's .claude/settings.json calls a
// guard as a PreToolUse hook; the tests put the vibe-guard they built on
// the probe runs' PATH. Only A-02 and A-03 run: the other parts would
// add minutes and decide nothing here.

// vibeGuardDir is a directory holding the vibe-guard this test binary
// built, once.
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
		return "", &buildError{out: string(out), err: err}
	}
	return dir, nil
})

type buildError struct {
	out string
	err error
}

func (e *buildError) Error() string { return "building vibe-guard: " + e.err.Error() + "\n" + e.out }

func builtVibeGuard(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	dir, err := vibeGuardDir()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// guardHooks is a .claude/settings.json whose PreToolUse hooks, one entry
// with the reference matcher, run commands.
func guardHooks(commands ...string) fixtureEdit {
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
	return withFile(".claude/settings.json", string(data)+"\n")
}

// referenceGuard is the reference PreToolUse command.
var referenceGuard = guard.ReferencePreToolUse("v0.0.0-test")

// guardPolicy is a .claude/hooks/policy.json.
func guardPolicy(body string) fixtureEdit {
	return withFile(".claude/hooks/policy.json", body)
}

// guardRules is the registry narrowed to ids, by default A-02 and A-03.
func guardRules(ids ...string) []contract.Rule {
	if len(ids) == 0 {
		ids = []string{"A-02", "A-03"}
	}
	var out []contract.Rule
	for _, r := range contract.Rules() {
		if slices.Contains(ids, r.ID) {
			out = append(out, r)
		}
	}
	return out
}

// auditGuard runs A-02 and A-03 against dir with extra directories first
// on the probe runs' PATH.
func auditGuard(t *testing.T, dir string, pathFirst ...string) (jsonReport, auditResult) {
	t.Helper()
	return auditGuardRules(t, guardRules(), dir, pathFirst...)
}

// auditGuardRules runs rules against dir, as auditGuard does.
func auditGuardRules(t *testing.T, rules []contract.Rule, dir string, pathFirst ...string) (jsonReport, auditResult) {
	t.Helper()
	path := os.Getenv("PATH")
	if len(pathFirst) > 0 {
		path = strings.Join(pathFirst, string(os.PathListSeparator)) + string(os.PathListSeparator) + path
	}
	env := probeEnvWithPath(t, path)
	if bash := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); bash != "" {
		env = append(env, "CLAUDE_CODE_GIT_BASH_PATH="+bash)
	}
	r := runVibeAuditSeams(t, contractSeams{rules: rules, probeEnv: env}, "--repo-root", dir, "--json")
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, r.stdout, r.stderr)
	}
	return rep, r
}

// assertProbe checks the result of a rule's probe part.
func assertProbe(t *testing.T, rep jsonReport, id, want string, findings ...string) {
	t.Helper()
	r := rep.rule(t, id)
	i := slices.IndexFunc(r.Parts, func(p struct {
		Check  string `json:"check"`
		Result string `json:"result"`
	}) bool {
		return p.Check == "probe"
	})
	if i < 0 {
		t.Fatalf("%s has no probe part: %+v", id, r.Parts)
	}
	if got := r.Parts[i].Result; got != want {
		t.Errorf("%s probe = %s, want %s\nfindings:\n%s", id, got, want, r.findingText())
	}
	text := r.findingText()
	for _, f := range findings {
		if !strings.Contains(text, f) {
			t.Errorf("%s findings missing %q\nfindings:\n%s", id, f, text)
		}
	}
}

func requireBash(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git is not on PATH, so Git Bash cannot be found")
		}
		return
	}
	if _, err := exec.LookPath("bash"); err != nil {
		if strings.Contains(os.Getenv("VIBE_TEST_REQUIRE_TOOLS"), "bash") {
			t.Fatal("bash is not on PATH and VIBE_TEST_REQUIRE_TOOLS requires it")
		}
		t.Skip("bash is not on PATH")
	}
}

// casesNamed returns the corpus case names A-02's findings name, in
// order.
func casesNamed(t *testing.T, rep jsonReport, id string) []string {
	t.Helper()
	var out []string
	for _, f := range rep.rule(t, id).Findings {
		rest, ok := strings.CutPrefix(f.Message, "corpus case ")
		if !ok {
			t.Errorf("%s finding names no corpus case: %s", id, f.Message)
			continue
		}
		var name string
		if err := json.NewDecoder(strings.NewReader(rest)).Decode(&name); err != nil {
			t.Errorf("%s finding: cannot read the case name: %s", id, f.Message)
			continue
		}
		out = append(out, name)
	}
	return out
}

// brokenGuard is a fixture guard that lets rm -r -f build through and
// hands every other call to vibe-guard.
const brokenGuard = `input=$(cat); case "$input" in *'"rm -r -f build"'*) exit 0;; esac; ` +
	`printf '%s' "$input" | vibe-guard hook`

// A-02 and A-03 with the reference guard and variants of it.
func TestRuleA02A03VibeGuard(t *testing.T) {
	t.Parallel()
	requireBash(t)
	bin := builtVibeGuard(t)
	t.Run("the reference guard passes A-02 and A-03", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditGuard(t, probeFixture(t, guardHooks(referenceGuard)), bin)
		assertProbe(t, rep, "A-02", "pass")
		assertProbe(t, rep, "A-03", "pass")
	})
	t.Run("a guard that allows rm -r -f fails A-02 and names the case", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditGuardRules(t, guardRules("A-02", "R-01"), probeFixture(t, guardHooks(brokenGuard)), bin)
		// The missed case is a deny case, so R-01's guard half fails too.
		assertExecution(t, rep, "R-01", "fail", `corpus case "rm -r -f (spec 0047)"`)
		assertProbe(t, rep, "A-02", "fail", `corpus case "rm -r -f (spec 0047)" (Bash "rm -r -f build") should be refused (guard rule rm-rf)`,
			"exited 0", "list an exception for rm-rf in .claude/hooks/policy.json")
		if got := casesNamed(t, rep, "A-02"); !slices.Equal(got, []string{"rm -r -f (spec 0047)"}) {
			t.Errorf("A-02 names %q, want only rm -r -f (spec 0047)", got)
		}
	})
	t.Run("observe mode fails A-02", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditGuardRules(t, guardRules("A-02"), probeFixture(t, guardHooks(referenceGuard),
			guardPolicy(`{"version": 2, "mode": "observe"}`+"\n")), bin)
		assertProbe(t, rep, "A-02", "fail", `corpus case "rm -r -f (spec 0047)"`, "exited 0")
	})
	t.Run("an exception lets its case through and is listed", func(t *testing.T) {
		t.Parallel()
		dir := probeFixture(t, guardHooks(referenceGuard), guardPolicy(`{"version": 2, "exceptions": [`+
			`{"rule": "rm-rf", "match": "rm -r -f build", "reason": "the build directory is disposable"}]}`+"\n"))
		rep, r := auditGuard(t, dir, bin)
		assertProbe(t, rep, "A-02", "pass")
		assertProbe(t, rep, "A-03", "pass")
		var uses struct {
			Uses []struct{ Rule, Match, Case string } `json:"guard_exception_uses"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &uses); err != nil {
			t.Fatal(err)
		}
		// The exception's words are a prefix of a simple command of each
		// of these cases, after wrappers and nested shells.
		var want []struct{ Rule, Match, Case string }
		for _, c := range []string{
			"rm -r -f (spec 0047)",
			"rm with extra blanks",
			"variant (#105): quoted flags",
			"variant (#105): sudo with options, by full paths",
			"variant (#105): sh -c with separate flags",
		} {
			want = append(want, struct{ Rule, Match, Case string }{"rm-rf", "rm -r -f build", c})
		}
		if !slices.Equal(uses.Uses, want) {
			t.Errorf("guard_exception_uses = %+v, want %+v", uses.Uses, want)
		}
	})
	t.Run("the text report lists the case under its exception", func(t *testing.T) {
		t.Parallel()
		dir := probeFixture(t, guardHooks(referenceGuard), guardPolicy(`{"version": 2, "exceptions": [`+
			`{"rule": "rm-rf", "match": "rm -r -f build", "reason": "the build directory is disposable"}]}`+"\n"))
		env := probeEnvWithPath(t, bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		r := runVibeAuditSeams(t, contractSeams{rules: guardRules("A-02"), probeEnv: env}, "--repo-root", dir)
		want := "Guard exceptions\n" +
			"  rm-rf \"rm -r -f build\" (the build directory is disposable)\n" +
			"    let through: rm -r -f (spec 0047)\n"
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the report lacks\n%s\nstdout:\n%s", want, r.stdout)
		}
	})
	t.Run("a hook without the install check fails A-03", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditGuardRules(t, guardRules("A-03"), probeFixture(t, guardHooks("vibe-guard hook")), bin)
		assertProbe(t, rep, "A-03", "fail", "with PATH reduced to the operating system's directories, ",
			`corpus case "rm -r -f (spec 0047)"`, "exited 127", "command -v vibe-guard")
		if text := rep.rule(t, "A-03").findingText(); strings.Contains(text, "replaced by `{`") {
			t.Errorf("A-03 fails the invalid-policy situation too:\n%s", text)
		}
	})
	t.Run("a guard that ignores an invalid policy fails A-03", func(t *testing.T) {
		t.Parallel()
		// The guard is replaced by allowing everything once the policy no
		// longer parses.
		hook := `grep -q '^{$' .claude/hooks/policy.json 2>/dev/null && exit 0; ` + referenceGuard
		rep, _ := auditGuardRules(t, guardRules("A-03"), probeFixture(t, guardHooks(hook), guardPolicy(`{"version": 2}`+"\n")), bin)
		assertProbe(t, rep, "A-03", "fail", "with .claude/hooks/policy.json replaced by `{`, ",
			"refuse every call (exit 2) when .claude/hooks/policy.json does not parse")
	})
	t.Run("R-01's guard half passes when A-02's deny cases are refused", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditGuardRules(t, guardRules("A-02", "R-01"), probeFixture(t, guardHooks(referenceGuard)), bin)
		assertExecution(t, rep, "R-01", "pass")
	})
	t.Run("no PreToolUse hook fails A-02 per tool", func(t *testing.T) {
		t.Parallel()
		rep, _ := auditGuardRules(t, guardRules("A-02"), probeFixture(t), bin)
		assertProbe(t, rep, "A-02", "fail", "no PreToolUse hook matches Bash, so its", "no PreToolUse hook matches Write")
	})
}

// The old guards of the repo-tooling modules run through the audit
// (spec 0053, "Old guards"). Each must satisfy every case #103 took over
// from their corpus; the cases #105 added that they miss are listed
// exactly. Only A-02 runs.
func TestRuleA02OldGuards(t *testing.T) {
	t.Parallel()
	requireBash(t)
	modules := filepath.Join("..", "module")
	// The cases #105 added that the old guards' regular expressions do
	// not refuse. The three guards share the regular expressions, so the
	// list is the same.
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
	policy, err := os.ReadFile(filepath.Join("..", "..", ".claude", "hooks", "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	guards := []struct {
		name    string
		tools   []string
		program string
	}{
		{"go", []string{"go"}, filepath.Join(modules, "repotooling", "templates", "guard.go")},
		{"python", pythonNames(), filepath.Join(modules, "pyrepotooling", "templates", "guard.py")},
		{"node", []string{"node"}, filepath.Join(modules, "tsrepotooling", "templates", "guard.mjs")},
	}
	for _, g := range guards {
		t.Run(g.name, func(t *testing.T) {
			t.Parallel()
			interp := requireToolOf(t, g.name, g.tools)
			program, err := filepath.Abs(g.program)
			if err != nil {
				t.Fatal(err)
			}
			var hook string
			if g.name == "go" {
				// go run reports the guard's exit 2 as 1.
				hook = "command -v go >/dev/null 2>&1 || exit 2; go run " + shellQuote(program) + " .claude/hooks/policy.json || exit 2"
			} else {
				hook = shellQuote(interp) + " " + shellQuote(program) + " .claude/hooks/policy.json || exit 2"
			}
			dir := probeFixture(t, guardHooks(hook), guardPolicy(string(policy)))
			rep, _ := auditGuardRules(t, guardRules("A-02"), dir)
			assertProbe(t, rep, "A-02", "fail")
			got := casesNamed(t, rep, "A-02")
			slices.Sort(got)
			want := slices.Clone(missed)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("the %s guard misses\n%q\nwant\n%q\nfindings:\n%s", g.name, got, want, rep.rule(t, "A-02").findingText())
			}
		})
	}
	t.Run("a tracked policy.local.json that loosens fails A-02", func(t *testing.T) {
		t.Parallel()
		requireToolOf(t, "go", []string{"go"})
		program, err := filepath.Abs(guards[0].program)
		if err != nil {
			t.Fatal(err)
		}
		hook := "command -v go >/dev/null 2>&1 || exit 2; go run " + shellQuote(program) + " .claude/hooks/policy.json || exit 2"
		dir := probeFixture(t, guardHooks(hook), guardPolicy(string(policy)),
			withFile(".claude/hooks/policy.local.json", `{"version": 1, "exceptions": [`+
				`{"rule": "rm-rf", "pattern": "rm -r -f build", "reason": "the build directory is disposable"}]}`+"\n"))
		rep, _ := auditGuardRules(t, guardRules("A-02"), dir)
		assertProbe(t, rep, "A-02", "fail", `corpus case "rm -r -f (spec 0047)"`,
			"exceptions in the tracked .claude/hooks/policy.local.json do not count")
	})
}

func pythonNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"python", "python3"}
	}
	return []string{"python3", "python"}
}

// requireToolOf returns the first of names on PATH, as a slash-separated
// path. Without one the test skips, unless VIBE_TEST_REQUIRE_TOOLS names
// gate.
func requireToolOf(t *testing.T, gate string, names []string) string {
	t.Helper()
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil && !strings.Contains(strings.ToLower(p), "windowsapps") {
			return filepath.ToSlash(p)
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
