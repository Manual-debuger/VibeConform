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

	"github.com/Manual-debuger/VibeConform/internal/contract"
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
// environment, with only the rules ids of the registry: a probe test
// waits only for the runs it reads, which keeps internal/cli within go
// test's ten-minute default. A rule that reads another's verdict (R-01
// reads A-02's when Claude Code is in use) needs both named.
func auditProbe(t *testing.T, dir string, ids []string, args ...string) jsonReport {
	t.Helper()
	r := runVibeAuditSeams(t, contractSeams{rules: registryRules(t, ids...)},
		append([]string{"--repo-root", dir, "--json"}, args...)...)
	var rep jsonReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("--json printed no JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, r.stdout, r.stderr)
	}
	return rep
}

// registryRules returns the registry's rules ids, in registry order.
func registryRules(t *testing.T, ids ...string) []contract.Rule {
	t.Helper()
	var rules []contract.Rule
	for _, r := range contract.Rules() {
		if slices.Contains(ids, r.ID) {
			rules = append(rules, r)
		}
	}
	if len(rules) != len(ids) {
		t.Fatalf("rules %v: some ID is not in the registry", ids)
	}
	return rules
}

// ids is a short way to write a list of rule IDs.
func ids(list ...string) []string { return list }

// assertD01 checks the result of D-01's execution part.
func assertD01(t *testing.T, rep jsonReport, want string, findings ...string) {
	t.Helper()
	const id, kind = "D-01", "execution"
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
	// The whole registry, the planted defects of V-01 and V-02 included.
	r := runVibeAuditSeams(t, contractSeams{}, "--repo-root", dir, "--json")
	if !strings.Contains(r.stdout, `"V-01"`) {
		t.Fatalf("the audit printed no report:\n%s\n%s", r.stdout, r.stderr)
	}
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
	// One probe part is enough to make the copy.
	r := runVibeAuditSeams(t, contractSeams{rules: registryRules(t, "V-03")}, "--repo-root", dir, "--keep-probe-copy")
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
	a07 := contractSeams{rules: registryRules(t, "A-07")}
	r := runVibeAuditSeams(t, a07, "--repo-root", dir)
	assertStderr(t, r, "vibe audit: the SessionStart hook")
	rj := runVibeAuditSeams(t, a07, "--repo-root", dir, "--json")
	if strings.Contains(rj.stderr, "vibe audit: the SessionStart hook") {
		t.Errorf("--json printed progress lines:\n%s", rj.stderr)
	}
}

// D-01: each component's lockfile matches its manifest.
func TestRuleD01(t *testing.T) {
	t.Parallel()
	// These cases need no tool: auditD01 runs D-01 alone with an empty
	// PATH. The cases that run go mod tidy are in internal/contract/probetest.
	auditD01 := func(t *testing.T, dir string) jsonReport {
		t.Helper()
		rep, _ := auditJSONRules(t, registryRules(t, "D-01"), dir)
		return rep
	}
	t.Run("no dependencies and no lockfile pass", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, contractFixture(t))
		assertD01(t, rep, "pass")
	})
	t.Run("a Python component with dependencies and no lockfile fails", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, contractFixture(t, withoutFile("go.mod"),
			withFile("pyproject.toml", "[project]\nname = \"svc\"\ndependencies = [\n  \"requests>=2\",  # http\n]\n")))
		assertD01(t, rep, "fail", "pyproject.toml: component . has dependencies", "uv.lock or poetry.lock")
	})
	t.Run("a Python component with only python in poetry passes", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, contractFixture(t, withoutFile("go.mod"),
			withFile("pyproject.toml", "[project]\nname = \"svc\"\ndependencies = []\n\n[tool.poetry.dependencies]\npython = \"^3.12\"\n")))
		assertD01(t, rep, "pass")
	})
	t.Run("a Python component with a poetry group fails", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, contractFixture(t, withoutFile("go.mod"),
			withFile("pyproject.toml", "[project]\nname = 'svc'\n\n[tool.poetry.group.dev.dependencies]\npytest = { version = \"^8\" }\n")))
		assertD01(t, rep, "fail", "has dependencies")
	})
	t.Run("a TypeScript component with devDependencies and no lockfile fails", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, contractFixture(t, withoutFile("go.mod"),
			withFile("package.json", `{"name": "web", "devDependencies": {"vitest": "^2"}}`+"\n")))
		assertD01(t, rep, "fail", "package.json: component . has dependencies", "pnpm-lock.yaml or package-lock.json")
	})
	t.Run("a lockfile check whose tool is not installed cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, contractFixture(t, withoutFile("go.mod"),
			withFile("package.json", `{"name": "web"}`+"\n"), withFile("pnpm-lock.yaml", "lockfileVersion: '9.0'\n")))
		assertD01(t, rep, "undecided", "`pnpm` is not installed on this machine")
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
