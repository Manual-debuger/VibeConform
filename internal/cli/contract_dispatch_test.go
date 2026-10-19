package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reportHeader starts the contract audit's report: a valid configuration
// reached the rule engine.
const reportHeader = "vibe audit: contract 1.0.1"

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
	dir := contractFixture(t)
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
	data, err := os.ReadFile(filepath.Join(baselineFixture, "vibe.yaml"))
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
// .git/config, like a dubious-ownership refusal, read as ErrNotWorkTree).
func TestContractAuditGitFailureIsNotNotWorkTree(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t)
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
	dir := contractFixture(t)
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
	dir := contractFixture(t, withFile("sub/vibe.yaml", "contract: 1.0.0\n"))
	r := runVibeAudit(t, "--repo-root", filepath.Join(dir, "sub"))
	assertExit(t, r, 1)
	assertStderr(t, r, "top level of its Git work tree")
}

func TestAuditDispatchBothKeys(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t,
		withFile("vibe.yaml", "contract: 1.0.0\nstandard: prod-go\nversion: v1\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "contract", "standard", "both")
}

func TestAuditDispatchNeitherKey(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, withFile("vibe.yaml", "platforms: [linux]\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "contract", "standard", "neither")
}

func TestAuditDispatchEmptyFileNamesBothKeys(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, withFile("vibe.yaml", "# nothing yet\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "contract", "standard", "neither")
}

func TestAuditDispatchNotAMapping(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, withFile("vibe.yaml", "- contract\n"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml:", "map")
}

// The old audit still runs on a v1 vibe.yaml: its own tests cover its
// output; this one only shows the dispatch reaches it.
func TestAuditDispatchV1RunsOldAudit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeManifest(t, dir)
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 2)
	if !strings.Contains(r.stdout, "standard: prod-go/v1") {
		t.Errorf("old audit did not run:\n%s", r.stdout)
	}
}

func TestAuditNewFlagsNeedV2(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--static", "--json"} {
		t.Run(flag, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir)
			r := runVibeAudit(t, "--repo-root", dir, flag)
			assertExit(t, r, 1)
			assertStderr(t, r, flag, "contract")
			if r.stdout != "" {
				t.Errorf("old audit ran despite %s:\n%s", flag, r.stdout)
			}
		})
	}
}

func TestAuditConfigMustNameV2File(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeManifest(t, dir)
	r := runVibeAudit(t, "--repo-root", dir, "--config", filepath.Join(dir, "vibe.yaml"))
	assertExit(t, r, 1)
	assertStderr(t, r, "--config", "contract")
	if r.stdout != "" {
		t.Errorf("old audit ran despite --config:\n%s", r.stdout)
	}
}

func TestAuditConfigSelectsV2Beside(t *testing.T) {
	t.Parallel()
	// A repository keeps its v1 vibe.yaml for the old audit and its v2
	// configuration beside it (spec 0052, "This repository").
	v2, err := os.ReadFile(filepath.Join(baselineFixture, "vibe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := contractFixture(t,
		withFile("vibe.yaml", "standard: prod-go\nversion: v1\n"),
		withFile("vibe.v2.yaml", string(v2)))

	r := runVibeAudit(t, "--repo-root", dir, "--config", filepath.Join(dir, "vibe.v2.yaml"))
	assertExit(t, r, 1)
	assertReport(t, r)
}

func TestAuditConfigErrorsNameTheConfigFile(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t,
		withFile("vibe.v2.yaml", "contract: 1.0.0\nplatforms: [bsd]\n"))
	cfg := filepath.Join(dir, "vibe.v2.yaml")
	r := runVibeAudit(t, "--repo-root", dir, "--config", cfg)
	assertExit(t, r, 1)
	assertStderr(t, r, cfg+": platforms[0]:")
}

// Without vibe.yaml and without a contract audit flag, the old audit
// reports the missing file as it always has.
func TestAuditMissingVibeYAMLRunsOldAudit(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, withoutFile("vibe.yaml"))
	r := runVibeAudit(t, "--repo-root", dir)
	assertExit(t, r, 1)
	assertStderr(t, r, "audit: open ", "vibe.yaml")
}

func TestAuditMissingVibeYAMLWithStatic(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t, withoutFile("vibe.yaml"))
	r := runVibeAudit(t, "--repo-root", dir, "--static")
	assertExit(t, r, 1)
	assertStderr(t, r, "vibe.yaml")
	if strings.Contains(r.stderr, "audit: open") {
		t.Errorf("--static fell back to the old audit:\n%s", r.stderr)
	}
}

func TestAuditConfigMissingFile(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t)
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
	t.Run("combined with another flag", func(t *testing.T) {
		for _, other := range [][]string{
			{"--static"}, {"--json"}, {"--config", "vibe.yaml"}, {"--repo-root", "."},
		} {
			r := runVibeAudit(t, append([]string{"--explain", "C-02"}, other...)...)
			assertExit(t, r, 1)
			assertStderr(t, r, "--explain", other[0])
		}
	})
	t.Run("reads no configuration", func(t *testing.T) {
		// Run where no vibe.yaml exists: --explain must not look for one.
		r := runVibeAudit(t, "--explain", "C-02")
		assertExit(t, r, 0)
		if strings.Contains(r.stderr, "vibe.yaml") {
			t.Errorf("--explain read a configuration:\n%s", r.stderr)
		}
	})
}
