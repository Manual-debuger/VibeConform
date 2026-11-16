package contract_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// Characterization of D-01's lockfiles (spec 0066, "Testing Decisions"):
// which commands each recognized lockfile runs and which fix it names.
// They pass on the code before the language records and must keep
// passing after them. No case needs a real tool: the probe PATH is empty
// or holds only fake programs.

// d01Lockfile is one lockfile D-01 recognizes, in a root component of
// its language, with the commands spec 0051 gives it.
type d01Lockfile struct {
	file string
	// edits make the baseline a root component that holds the lockfile.
	edits []contracttest.Edit
	// first is the first command of the lockfile's check, with its
	// directory, as the finding names it.
	first string
	// program is the first command's program.
	program string
	// fix is the command the stale-lockfile fix names.
	fix string
}

func d01Lockfiles() []d01Lockfile {
	python := func(lockfile string) []contracttest.Edit {
		return []contracttest.Edit{
			contracttest.WithoutFile("go.mod"),
			contracttest.WithFile("pyproject.toml", "[project]\nname = \"svc\"\ndependencies = [\"requests>=2\"]\n"),
			contracttest.WithFile(lockfile, "# lockfile\n"),
		}
	}
	typescript := func(lockfile, content string) []contracttest.Edit {
		return []contracttest.Edit{
			contracttest.WithoutFile("go.mod"),
			contracttest.WithFile("package.json", `{"name": "web", "devDependencies": {"vitest": "^2"}}`+"\n"),
			contracttest.WithFile(lockfile, content),
		}
	}
	return []d01Lockfile{
		{
			file:    "go.sum",
			edits:   []contracttest.Edit{contracttest.WithFile("go.sum", "")},
			first:   "go mod tidy in .",
			program: "go",
			fix:     "go mod tidy",
		},
		{
			file:    "uv.lock",
			edits:   python("uv.lock"),
			first:   "uv lock --check in .",
			program: "uv",
			fix:     "uv lock",
		},
		{
			file:    "poetry.lock",
			edits:   python("poetry.lock"),
			first:   "poetry check --lock in .",
			program: "poetry",
			fix:     "poetry lock",
		},
		{
			file:    "pnpm-lock.yaml",
			edits:   typescript("pnpm-lock.yaml", "lockfileVersion: '9.0'\n"),
			first:   "pnpm install --frozen-lockfile --lockfile-only in .",
			program: "pnpm",
			fix:     "pnpm install",
		},
		{
			file:    "package-lock.json",
			edits:   typescript("package-lock.json", `{"lockfileVersion": 3}`+"\n"),
			first:   "npm ci --dry-run in .",
			program: "npm",
			fix:     "npm install",
		},
	}
}

// auditD01 runs D-01 alone on the baseline with edits, with env as the
// probe environment.
func auditD01(t *testing.T, env []string, edits ...contracttest.Edit) *contract.Report {
	t.Helper()
	return contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline, edits...), contract.Options{
		Rules: contracttest.Rules(t, "D-01"),
		Probe: contract.ProbeOptions{Env: env},
	})
}

// fakeProgramPath is a probe PATH: a new directory that holds a fake
// program for each name, which prints "fake <name> exits <code>" and
// exits with code. The fake is a shell script, or a batch file on
// Windows. A finding that quotes the line shows that the fake ran.
func fakeProgramPath(t *testing.T, code map[string]int) string {
	t.Helper()
	dir := t.TempDir()
	for name, exit := range code {
		line := "fake " + name + " exits " + strconv.Itoa(exit)
		file, content := filepath.Join(dir, name), "#!/bin/sh\necho '"+line+"'\nexit "+strconv.Itoa(exit)+"\n"
		if runtime.GOOS == "windows" {
			file, content = file+".bat", "@echo "+line+"\r\n@exit /b "+strconv.Itoa(exit)+"\r\n"
		}
		// #nosec G306 -- the fake program must be executable
		if err := os.WriteFile(file, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// With no tool installed, each lockfile's check cannot decide and names
// its first command: this pins which commands each lockfile runs.
func TestRuleD01EachLockfileRunsItsCheck(t *testing.T) {
	t.Parallel()
	for _, lf := range d01Lockfiles() {
		t.Run(lf.file, func(t *testing.T) {
			t.Parallel()
			rep := auditD01(t, contracttest.EmptyProbeEnv(t), lf.edits...)
			contracttest.AssertPart(t, rep, "D-01", contract.Execution, contract.ResultUndecided,
				lf.file+": cannot decide: "+lf.first,
				"`"+lf.program+"` is not installed on this machine")
		})
	}
}

// When a lockfile's check exits non-zero, D-01 fails on that lockfile and
// its fix names the lockfile's own fix command.
func TestRuleD01StaleLockfileFix(t *testing.T) {
	t.Parallel()
	for _, lf := range d01Lockfiles() {
		t.Run(lf.file, func(t *testing.T) {
			t.Parallel()
			env := contracttest.ProbeEnvWithPath(fakeProgramPath(t, map[string]int{lf.program: 1}))
			rep := auditD01(t, env, lf.edits...)
			contracttest.AssertPart(t, rep, "D-01", contract.Execution, contract.ResultFail,
				lf.file+": does not match its manifest: "+lf.first,
				"exited 1", "fake "+lf.program+" exits 1",
				"Fix: update the lockfile with the package manager, such as "+lf.fix+", and commit it\n")
		})
	}
	// go.sum's check runs a second command: git diff on go.mod and go.sum
	// after go mod tidy.
	t.Run("go.sum when git diff finds a change", func(t *testing.T) {
		t.Parallel()
		env := contracttest.ProbeEnvWithPath(fakeProgramPath(t, map[string]int{"go": 0, "git": 1}))
		rep := auditD01(t, env, contracttest.WithFile("go.sum", ""))
		contracttest.AssertPart(t, rep, "D-01", contract.Execution, contract.ResultFail,
			"go.sum: does not match its manifest: git diff --exit-code go.mod go.sum in .",
			"fake git exits 1",
			"Fix: update the lockfile with the package manager, such as go mod tidy, and commit it\n")
	})
}

// D-01 checks every recognized lockfile in a component's directory,
// whatever the component's language (spec 0066, Assumption 1): a Go
// component with a stray package-lock.json runs npm's check.
func TestRuleD01StrayLockfileOfAnotherLanguage(t *testing.T) {
	t.Parallel()
	t.Run("npm's check runs", func(t *testing.T) {
		t.Parallel()
		rep := auditD01(t, contracttest.EmptyProbeEnv(t), contracttest.WithFile("package-lock.json", `{"lockfileVersion": 3}`+"\n"))
		contracttest.AssertPart(t, rep, "D-01", contract.Execution, contract.ResultUndecided,
			"package-lock.json: cannot decide: npm ci --dry-run in .", "`npm` is not installed on this machine")
	})
	t.Run("its failure names npm's fix", func(t *testing.T) {
		t.Parallel()
		env := contracttest.ProbeEnvWithPath(fakeProgramPath(t, map[string]int{"npm": 1}))
		rep := auditD01(t, env, contracttest.WithFile("package-lock.json", `{"lockfileVersion": 3}`+"\n"))
		contracttest.AssertPart(t, rep, "D-01", contract.Execution, contract.ResultFail,
			"package-lock.json: does not match its manifest: npm ci --dry-run in .",
			"fake npm exits 1", "such as npm install, and commit it")
	})
}

// A Go component with dependencies and no go.sum names go.sum, the
// lockfiles of its language. TestRuleD01 has the Python and TypeScript
// cases.
func TestRuleD01NoLockfileNamesTheLanguagesLockfiles(t *testing.T) {
	t.Parallel()
	rep := auditD01(t, contracttest.EmptyProbeEnv(t),
		contracttest.WithFile("go.mod", "module example.com/fixture\n\ngo 1.27.0\n\nrequire example.com/lib v0.0.0\n"))
	contracttest.AssertPart(t, rep, "D-01", contract.Execution, contract.ResultFail,
		"go.mod: component . has dependencies and no recognized lockfile (go.sum)",
		"Fix: commit the lockfile your package manager writes, such as go.sum\n")
}
