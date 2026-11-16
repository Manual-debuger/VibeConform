package cli

import (
	"path/filepath"
	"regexp"
	"testing"
)

// The messages for a missing or version 1 vibe.yaml are part of what an
// adopter sees, and loading the configuration in internal/contract (spec
// 0065) changes none of them. These cases hold the full text, written out
// here rather than built the way the code builds it.

// wantMigrationPointer is the migration pointer's text (spec 0059,
// Desired Behavior 8 and 9).
const wantMigrationPointer = "to leave the template-sync system, run `vibe init --skill` and follow the migration guide, " +
	".claude/skills/vibeconform-setup/reference/migrate.md"

// wantV1 is the error of audit and doctor for a version 1 file named file.
func wantV1(file string) string {
	return file + " is version 1 (a top-level standard key), which this vibe no longer reads; " + wantMigrationPointer
}

// wantNoConfig is the error of audit and doctor when root has no vibe.yaml.
func wantNoConfig(root string) string {
	return "no vibe.yaml in " + root + ": run `vibe init` to write one"
}

const v1Config = "standard: prod-go\nversion: v1\n"

func TestAuditConfigMessagesAreUnchanged(t *testing.T) {
	t.Parallel()
	t.Run("no vibe.yaml", func(t *testing.T) {
		t.Parallel()
		dir := contractFixture(t, withoutFile("vibe.yaml"))
		r := runVibeAudit(t, "--repo-root", dir)
		assertExit(t, r, exitError)
		if want := "Error: " + wantNoConfig(dir) + "\n"; r.stderr != want {
			t.Errorf("stderr:\n got %q\nwant %q", r.stderr, want)
		}
	})
	t.Run("version 1", func(t *testing.T) {
		t.Parallel()
		dir := contractFixture(t, withFile("vibe.yaml", v1Config))
		r := runVibeAudit(t, "--repo-root", dir)
		assertExit(t, r, exitError)
		if want := "Error: " + wantV1("vibe.yaml") + "\n"; r.stderr != want {
			t.Errorf("stderr:\n got %q\nwant %q", r.stderr, want)
		}
	})
	t.Run("version 1 by --config", func(t *testing.T) {
		t.Parallel()
		dir := contractFixture(t, withFile("old.yaml", v1Config))
		cfg := filepath.Join(dir, "old.yaml")
		r := runVibeAudit(t, "--repo-root", dir, "--config", cfg)
		assertExit(t, r, exitError)
		if want := "Error: " + wantV1(cfg) + "\n"; r.stderr != want {
			t.Errorf("stderr:\n got %q\nwant %q", r.stderr, want)
		}
	})
}

// Doctor prints the audit's message on its vibe.yaml line, and exits 1.
// It changes doctorEnv, so it does not run in parallel.
func TestDoctorConfigMessagesAreUnchanged(t *testing.T) {
	for _, c := range []struct {
		name  string
		edits []fixtureEdit
		want  func(dir string) string
	}{
		{"no vibe.yaml", []fixtureEdit{withoutFile("vibe.yaml")}, wantNoConfig},
		{"version 1", []fixtureEdit{withFile("vibe.yaml", v1Config)}, func(string) string { return wantV1("vibe.yaml") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := contractFixture(t, c.edits...)
			stubDoctorEnv(t)
			out, err := runDoctorIn(t, dir)
			if code := ExitCode(err); code != exitError {
				t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
			}
			assertLines(t, out, `^FAIL +vibe\.yaml +`+regexp.QuoteMeta(c.want(dir))+`$`)
		})
	}
}

func TestInitRefusalMessagesAreUnchanged(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, content, want string
	}{
		{"version 1", v1Config, "init: vibe.yaml exists and is version 1 (a top-level standard key); " +
			"vibe init does not overwrite it. " + wantMigrationPointer},
		{"version 2", "contract: 1.1.0\nplatforms: [linux]\nagents: []\n",
			"init: vibe.yaml exists; vibe init does not overwrite it. Edit it, or remove it and run vibe init again"},
		{"neither key", "platforms: [linux]\n",
			"init: vibe.yaml exists; vibe init does not overwrite it. Edit it, or remove it and run vibe init again"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := contractFixture(t, withFile("vibe.yaml", c.content))
			_, err := runInit(t, dir)
			if err == nil {
				t.Fatal("vibe init wrote over an existing vibe.yaml")
			}
			if got := err.Error(); got != c.want {
				t.Errorf("error:\n got %q\nwant %q", got, c.want)
			}
		})
	}
}
