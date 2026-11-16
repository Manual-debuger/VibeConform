package contract_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The execution and probe parts (spec 0053), through contract.Audit with
// real processes. The Go probe fixture needs only Go: its entrypoints and
// hooks run go run ./ci <step>. The flags that set ProbeOptions are
// tested through the vibe audit command in internal/cli.

// probeFixture builds a repository from the Go probe fixture with edits.
func probeFixture(t *testing.T, edits ...contracttest.Edit) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	return contracttest.Fixture(t, probeGoFixture, edits...)
}

// probeSessionStartHooks is a .claude/settings.json with SessionStart
// hooks that run commands.
func probeSessionStartHooks(matcher string, commands ...string) contracttest.Edit {
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
	return contracttest.WithFile(".claude/settings.json", string(data)+"\n")
}

// probeGoHook guards a go run hook command the way the spec's fixtures
// do.
func probeGoHook(step string) string {
	return "command -v go >/dev/null 2>&1 || exit 2; go run ./ci " + step
}

// probeTreeHash hashes every file under dir, .git included, by path and
// content.
func probeTreeHash(t *testing.T, dir string) map[string]string {
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

// probeTempEntries lists the entries of the operating system's temporary
// directory whose names start with prefix.
func probeTempEntries(t *testing.T, dir, prefix string) []string {
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

// probeSetTempDir points the operating system's temporary directory at a
// new directory for this test.
func probeSetTempDir(t *testing.T) string {
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
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	dir := probeFixture(t,
		// An untracked and an ignored file stay out of the copy and stay
		// where they are.
		contracttest.WithFile(".gitignore", "build/\n"),
		contracttest.WithUntracked("notes.txt", "mine\n"),
		contracttest.WithUntracked("build/out.bin", "artifact\n"),
		probeSessionStartHooks("startup", probeGoHook("session-start-write")),
	)
	tmp := probeSetTempDir(t)
	before := probeTreeHash(t, dir)
	// The whole registry, the planted defects of V-01 and V-02 included,
	// in this process's environment.
	rep := contracttest.Audit(t, dir, contract.Options{})
	contracttest.RuleResult(t, rep, "V-01")
	after := probeTreeHash(t, dir)
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
	if left := probeTempEntries(t, tmp, "vibe-audit-"); len(left) > 0 {
		t.Errorf("the probe copy was not deleted: %v", left)
	}
}

func TestContractAuditKeepProbeCopy(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	dir := probeFixture(t)
	tmp := probeSetTempDir(t)
	// One probe part is enough to make the copy.
	var stderr bytes.Buffer
	contracttest.Audit(t, dir, contract.Options{
		Rules: contracttest.Rules(t, "V-03"),
		Probe: contract.ProbeOptions{KeepCopy: true, Stderr: &stderr},
	})
	if !strings.Contains(stderr.String(), "vibe audit: keeping the probe copy at ") {
		t.Errorf("stderr lacks the kept copy's path:\n%s", stderr.String())
	}
	left := probeTempEntries(t, tmp, "vibe-audit-")
	if len(left) != 1 {
		t.Fatalf("temporary directory holds %v, want one kept probe copy", left)
	}
	if _, err := os.Stat(filepath.Join(tmp, left[0], "copy", "calc", "calc_test.go")); err != nil {
		t.Errorf("kept copy lacks a tracked file: %v", err)
	}
}

// Progress lines go to ProbeOptions.Progress only: the CLI passes it in
// text mode, and --json passes Stderr alone, which gets none.
func TestContractAuditProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	dir := probeFixture(t)
	a07 := contracttest.Rules(t, "A-07")
	var progress, stderr bytes.Buffer
	contracttest.Audit(t, dir, contract.Options{Rules: a07, Probe: contract.ProbeOptions{Progress: &progress, Stderr: &stderr}})
	if !strings.Contains(progress.String(), "vibe audit: the SessionStart hook") {
		t.Errorf("progress lacks the SessionStart hook's run:\n%s", progress.String())
	}
	var jsonStderr bytes.Buffer
	contracttest.Audit(t, dir, contract.Options{Rules: a07, Probe: contract.ProbeOptions{Stderr: &jsonStderr}})
	if strings.Contains(stderr.String()+jsonStderr.String(), "vibe audit: the SessionStart hook") {
		t.Errorf("Stderr got progress lines:\n%s\n%s", stderr.String(), jsonStderr.String())
	}
}

// D-01: each component's lockfile matches its manifest.
func TestRuleD01(t *testing.T) {
	t.Parallel()
	// These cases need no tool: auditD01 runs D-01 alone with an empty
	// PATH. The cases that run go mod tidy are in internal/contract/probetest.
	auditD01 := func(t *testing.T, dir string) *contract.Report {
		t.Helper()
		return contracttest.Audit(t, dir, contract.Options{
			Rules: contracttest.Rules(t, "D-01"),
			Probe: contract.ProbeOptions{Env: contracttest.EmptyProbeEnv(t)},
		})
	}
	assertD01 := func(t *testing.T, rep *contract.Report, want contract.Result, findings ...string) {
		t.Helper()
		contracttest.AssertPart(t, rep, "D-01", contract.Execution, want, findings...)
	}
	baseline := func(t *testing.T, edits ...contracttest.Edit) string {
		t.Helper()
		return contracttest.Fixture(t, contracttest.Baseline, edits...)
	}
	t.Run("no dependencies and no lockfile pass", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, baseline(t))
		assertD01(t, rep, contract.ResultPass)
	})
	t.Run("a Python component with dependencies and no lockfile fails", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, baseline(t, contracttest.WithoutFile("go.mod"),
			contracttest.WithFile("pyproject.toml", "[project]\nname = \"svc\"\ndependencies = [\n  \"requests>=2\",  # http\n]\n")))
		assertD01(t, rep, contract.ResultFail, "pyproject.toml: component . has dependencies", "uv.lock or poetry.lock")
	})
	t.Run("a Python component with only python in poetry passes", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, baseline(t, contracttest.WithoutFile("go.mod"),
			contracttest.WithFile("pyproject.toml", "[project]\nname = \"svc\"\ndependencies = []\n\n[tool.poetry.dependencies]\npython = \"^3.12\"\n")))
		assertD01(t, rep, contract.ResultPass)
	})
	t.Run("a Python component with a poetry group fails", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, baseline(t, contracttest.WithoutFile("go.mod"),
			contracttest.WithFile("pyproject.toml", "[project]\nname = 'svc'\n\n[tool.poetry.group.dev.dependencies]\npytest = { version = \"^8\" }\n")))
		assertD01(t, rep, contract.ResultFail, "has dependencies")
	})
	t.Run("a TypeScript component with devDependencies and no lockfile fails", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, baseline(t, contracttest.WithoutFile("go.mod"),
			contracttest.WithFile("package.json", `{"name": "web", "devDependencies": {"vitest": "^2"}}`+"\n")))
		assertD01(t, rep, contract.ResultFail, "package.json: component . has dependencies", "pnpm-lock.yaml or package-lock.json")
	})
	t.Run("a lockfile check whose tool is not installed cannot decide", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, baseline(t, contracttest.WithoutFile("go.mod"),
			contracttest.WithFile("package.json", `{"name": "web"}`+"\n"), contracttest.WithFile("pnpm-lock.yaml", "lockfileVersion: '9.0'\n")))
		assertD01(t, rep, contract.ResultUndecided, "`pnpm` is not installed on this machine")
	})
}
