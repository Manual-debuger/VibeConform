package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// reportHeader starts the contract audit's report: a valid configuration
// reached the rule engine.
const reportHeader = "vibe audit: contract 1.2.0"

// assertReport checks that the contract audit printed its report.
func assertReport(t *testing.T, r auditResult) {
	t.Helper()
	if !strings.HasPrefix(r.stdout, reportHeader) {
		t.Errorf("no contract audit report on stdout\nstdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
}

func assertStderr(t *testing.T, r auditResult, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(r.stderr, w) {
			t.Errorf("stderr missing %q\nstderr:\n%s\nstdout:\n%s", w, r.stderr, r.stdout)
		}
	}
}

func assertExit(t *testing.T, r auditResult, want int) {
	t.Helper()
	if r.code != want {
		t.Errorf("exit code = %d, want %d\nstderr:\n%s\nstdout:\n%s", r.code, want, r.stderr, r.stdout)
	}
}

func TestContractAuditValidConfigReachesEngine(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline)
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertReport(t, r)
	assertStderr(t, r, "cannot decide")
	if strings.Contains(r.stderr, "vibe.yaml:") {
		t.Errorf("valid baseline reported a configuration error:\n%s", r.stderr)
	}
}

func TestContractAuditOutsideWorkTree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join(contracttest.Baseline, "vibe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeVibeYAML(t, dir, string(data))

	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "not inside a Git work tree")
}

// A git failure inside a work tree is a tool error that says what git
// said, not "not inside a Git work tree" (regression: a broken
// .git/config, like a dubious-ownership refusal, read as not a work tree).
func TestContractAuditGitFailureIsNotNotWorkTree(t *testing.T) {
	t.Parallel()
	dir := contracttest.GitFixture(t, contracttest.Baseline)
	cfg := filepath.Join(dir, ".git", "config")
	f, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("[broken\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "git rev-parse --is-inside-work-tree", "bad config")
	if strings.Contains(r.stderr, "not inside a Git work tree") {
		t.Errorf("a git failure was reported as not a work tree:\n%s", r.stderr)
	}
}

// A clone with no refs/remotes/origin/HEAD falls back to main or master
// with a note; a git failure reading it is a tool error, not that
// fallback (regression: every error read as "no origin/HEAD").
func TestContractAuditOriginHEADGitFailure(t *testing.T) {
	t.Parallel()
	dir := contracttest.GitFixture(t, contracttest.Baseline)
	ref := filepath.Join(dir, ".git", "refs", "remotes", "origin", "HEAD")
	if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ref, []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := runVibeAudit(t, "--repo-root", dir, "--static")
	assertExit(t, r, 1)
	assertStderr(t, r, "git symbolic-ref", "refs/remotes/origin/HEAD")
	if strings.Contains(r.stdout, "assume the default branch") {
		t.Errorf("a git failure fell back to an assumed default branch:\n%s", r.stdout)
	}
}

func TestContractAuditRepoRootMustBeTopLevel(t *testing.T) {
	t.Parallel()
	dir := contracttest.GitFixture(t, contracttest.Baseline, contracttest.WithFile("sub/vibe.yaml", "contract: 1.0.0\n"))
	r := runVibeAudit(t, "--repo-root", filepath.Join(dir, "sub"))
	assertExit(t, r, 1)
	assertStderr(t, r, "top level of its Git work tree")
}

func TestAuditDispatchBothKeys(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline,
		contracttest.WithFile("vibe.yaml", "contract: 1.0.0\nstandard: prod-go\nversion: v1\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "contract", "standard", "both")
}

func TestAuditDispatchNeitherKey(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", "platforms: [linux]\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "contract", "standard", "neither")
}

func TestAuditDispatchEmptyFileNamesBothKeys(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", "# nothing yet\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "contract", "standard", "neither")
}

func TestAuditDispatchNotAMapping(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", "- contract\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "map")
}

// A version 1 vibe.yaml (a top-level standard key) is no longer read: the
// audit exits 1 and points at the migration guide, whatever flags it got
// (spec 0059, Desired Behavior 9).
func TestAuditV1PointsAtTheMigrationGuide(t *testing.T) {
	t.Parallel()
	for _, flags := range [][]string{nil, {"--static"}, {"--json"}, {"--config", "vibe.yaml"}} {
		t.Run(strings.Join(append([]string{"audit"}, flags...), " "), func(t *testing.T) {
			dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", "standard: prod-go\nversion: v1\n"))
			args := []string{"--repo-root", dir}
			if len(flags) == 2 {
				args = append(args, flags[0], filepath.Join(dir, flags[1]))
			} else {
				args = append(args, flags...)
			}
			r := runVibeAudit(t, args...)
			assertExit(t, r, 1)
			assertStderr(t, r, "vibe.yaml", "version 1", "vibe init --skill", migrationGuide)
			if r.stdout != "" {
				t.Errorf("an audit ran on a version 1 file:\n%s", r.stdout)
			}
		})
	}
}

func TestAuditConfigSelectsV2Beside(t *testing.T) {
	t.Parallel()
	// --config reads only the file it names, whatever vibe.yaml holds.
	v2, err := os.ReadFile(filepath.Join(contracttest.Baseline, "vibe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := contracttest.Fixture(t, contracttest.Baseline,
		contracttest.WithFile("vibe.yaml", "standard: prod-go\nversion: v1\n"),
		contracttest.WithFile("vibe.v2.yaml", string(v2)))

	r := runVibeAudit(t, "--repo-root", dir, "--config", filepath.Join(dir, "vibe.v2.yaml"))
	assertExit(t, r, 1)
	assertReport(t, r)
}

func TestAuditConfigErrorsNameTheConfigFile(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline,
		contracttest.WithFile("vibe.v2.yaml", "contract: 1.0.0\nplatforms: [bsd]\n"))
	cfg := filepath.Join(dir, "vibe.v2.yaml")
	r := runVibeAudit(t, "--repo-root", dir, "--config", cfg)
	assertExit(t, r, 1)
	assertStderr(t, r, cfg+": platforms[0]:")
}

// Without vibe.yaml the audit exits 1 and says to run vibe init, with or
// without a contract audit flag (spec 0059, Desired Behavior 9).
func TestAuditMissingVibeYAMLSaysRunInit(t *testing.T) {
	t.Parallel()
	for _, flags := range [][]string{nil, {"--static"}, {"--json"}} {
		t.Run(strings.Join(append([]string{"audit"}, flags...), " "), func(t *testing.T) {
			dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithoutFile("vibe.yaml"))
			r := runVibeAudit(t, append([]string{"--repo-root", dir}, flags...)...)
			assertExit(t, r, 1)
			assertStderr(t, r, "vibe.yaml", "run `vibe init`")
			if r.stdout != "" {
				t.Errorf("an audit ran without vibe.yaml:\n%s", r.stdout)
			}
		})
	}
}

func TestAuditConfigMissingFile(t *testing.T) {
	t.Parallel()
	dir := contracttest.Fixture(t, contracttest.Baseline)
	r := runVibeAudit(t, "--repo-root", dir, "--config", filepath.Join(dir, "absent.yaml"))
	assertExit(t, r, 1)
	assertStderr(t, r, "absent.yaml")
}

func TestAuditExplain(t *testing.T) {
	t.Parallel()
	t.Run("unknown ID lists the valid ones", func(t *testing.T) {
		r := runVibeAudit(t, "--explain", "Z-99")
		assertExit(t, r, 1)
		assertStderr(t, r, "Z-99", "V-01", "R-03")
	})
	t.Run("combined with a flag that does not name the configuration", func(t *testing.T) {
		for _, other := range [][]string{
			{"--static"}, {"--json"}, {"--probe-timeout", "1m"}, {"--keep-probe-copy"},
		} {
			r := runVibeAudit(t, append([]string{"--explain", "C-02"}, other...)...)
			assertExit(t, r, 1)
			assertStderr(t, r, "--explain", other[0])
		}
	})
	t.Run("several other flags are named in cobra's order", func(t *testing.T) {
		r := runVibeAudit(t, "--static", "--explain", "C-02", "--repo-root", ".", "--json")
		assertExit(t, r, 1)
		assertStderr(t, r, "--explain reads only the contract version of the configuration; drop --json, --static\n")
	})
	t.Run("a flag that no list holds", func(t *testing.T) {
		// The flag exists only in this command tree, so a refusal proves
		// that --explain visits the set flags and keeps no list of them.
		root := newRootCmd("test", cliSeams{openRepo: contracttest.OpenRepo})
		audit, _, err := root.Find([]string{"audit"})
		if err != nil {
			t.Fatal(err)
		}
		audit.Flags().Bool("added-in-test", false, "a flag vibe audit does not have")
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs([]string{"audit", "--explain", "C-02", "--added-in-test"})
		r := auditResult{code: ExitCode(root.Execute()), stdout: out.String(), stderr: errOut.String()}
		assertExit(t, r, 1)
		assertStderr(t, r, "--explain reads only the contract version of the configuration; drop --added-in-test\n")
		if r.stdout != "" {
			t.Errorf("--explain printed the rule although it refused:\n%s", r.stdout)
		}
	})
}

// --explain prints a rule as it is under the contract the configuration
// declares, or under the latest when there is no vibe.yaml (spec 0074).
// It reads only the contract field: the rest of the file may be invalid.
func TestAuditExplainReadsTheContractVersion(t *testing.T) {
	t.Parallel()
	withConfig := func(t *testing.T, body string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	flat := func(r auditResult) string { return strings.Join(strings.Fields(r.stdout), " ") }
	assertShows := func(t *testing.T, r auditResult, want ...string) {
		t.Helper()
		assertExit(t, r, 0)
		for _, w := range want {
			if !strings.Contains(flat(r), w) {
				t.Errorf("--explain does not show %q:\n%s", w, r.stdout)
			}
		}
	}
	const c05Of1 = "C-05: Tests run on Linux and Windows"
	const c05Of2 = "C-05: Tests run on every declared platform"

	t.Run("a 1.x configuration", func(t *testing.T) {
		t.Parallel()
		dir := withConfig(t, "contract: 1.0.0\nplatforms: [nonsense]\n")
		r := runVibeAudit(t, "--explain", "C-05", "--repo-root", dir)
		assertShows(t, r, c05Of1, "Contract: 1.2.0 (vibe.yaml declares 1.0.0)",
			"Not applicable: when `platforms` does not include `windows`",
			"Static: the steps that call `verify` run on a Linux runner and on a Windows runner")
	})
	t.Run("a 2.0.x configuration is read as 2.1.0", func(t *testing.T) {
		t.Parallel()
		dir := withConfig(t, "contract: 2.0.0\n")
		r := runVibeAudit(t, "--explain", "C-05", "--repo-root", dir)
		assertShows(t, r, c05Of2, "Contract: 2.1.0 (vibe.yaml declares 2.0.0)", "Not applicable: never",
			"Static: the steps that call `verify` run on a runner of each platform in `platforms`")
	})
	t.Run("--config names the file", func(t *testing.T) {
		t.Parallel()
		dir := withConfig(t, "contract: 2.1.0\n")
		config := filepath.Join(withConfig(t, "contract: 1.2.0\n"), "vibe.yaml")
		r := runVibeAudit(t, "--explain", "C-05", "--repo-root", dir, "--config", config)
		assertShows(t, r, c05Of1, "Contract: 1.2.0 ("+config+" declares 1.2.0)")
	})
	t.Run("no vibe.yaml means the latest contract", func(t *testing.T) {
		t.Parallel()
		r := runVibeAudit(t, "--explain", "C-05", "--repo-root", t.TempDir())
		assertShows(t, r, c05Of2, "Contract: 2.1.0, the latest (no vibe.yaml)")
		if r.stderr != "" {
			t.Errorf("stderr not empty:\n%s", r.stderr)
		}
	})
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"a contract this vibe does not implement", "contract: 3.0.0\n", []string{"vibe.yaml", "upgrade vibe"}},
		{"a configuration without a contract", "platforms: [linux]\n", []string{"vibe.yaml", "declare the contract version"}},
		{"a version 1 configuration", "standard: vibe\n", []string{"vibe.yaml is version 1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := runVibeAudit(t, "--explain", "C-05", "--repo-root", withConfig(t, tc.body))
			assertExit(t, r, 1)
			assertStderr(t, r, tc.want...)
			if r.stdout != "" {
				t.Errorf("--explain printed the rule although it cannot tell the contract:\n%s", r.stdout)
			}
		})
	}
}
