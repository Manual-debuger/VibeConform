package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// stubDoctorEnv makes doctor see a machine where every binary is on PATH
// except those named missing, every command succeeds, and bash resolves
// unless "bash" is missing. Files are the real ones, since the repository
// under test is the test's own temp directory.
func stubDoctorEnv(t *testing.T, missing ...string) {
	t.Helper()
	previous := doctorEnv
	doctorEnv = func() doctor.Env {
		return doctor.Env{
			LookPath: func(name string) (string, error) {
				if slices.Contains(missing, name) {
					return "", exec.ErrNotFound
				}
				return "/stub/" + name, nil
			},
			Run: func(_ context.Context, _, _ string, args ...string) (string, string, error) {
				return "v1\n", "", nil
			},
			Exists: func(path string) bool {
				_, err := os.Stat(path)
				return err == nil
			},
			Bash: func() (string, error) {
				if slices.Contains(missing, "bash") {
					return "", errors.New("no bash found")
				}
				return "/stub/bash", nil
			},
		}
	}
	t.Cleanup(func() { doctorEnv = previous })
}

func runDoctorIn(t *testing.T, dir string) (string, error) {
	t.Helper()
	root := newRootCmd("test", contractSeams{openRepo: openFixtureRepo})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doctor", "--repo-root", dir})
	err := root.Execute()
	return out.String(), err
}

// assertLines checks that every pattern matches a line of out.
func assertLines(t *testing.T, out string, patterns ...string) {
	t.Helper()
	for _, p := range patterns {
		if !regexp.MustCompile(`(?m)` + p).MatchString(out) {
			t.Errorf("output does not match %s:\n%s", p, out)
		}
	}
}

// A version 1 file and a missing file get the messages vibe audit gives
// them, and exit 1 (spec 0059, Desired Behavior 11).
func TestDoctorConfigurationLikeAudit(t *testing.T) {
	for _, c := range []struct {
		name  string
		edits []fixtureEdit
	}{
		{"no file", []fixtureEdit{withoutFile("vibe.yaml")}},
		{"version 1", []fixtureEdit{withFile("vibe.yaml", "standard: prod-go\nversion: v1\n")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := contractFixture(t, c.edits...)
			audit := runVibeAudit(t, "--repo-root", dir)
			msg := strings.TrimSpace(strings.TrimPrefix(audit.stderr, "Error: "))
			if audit.code != exitError || msg == "" {
				t.Fatalf("vibe audit exit %d, stderr %q", audit.code, audit.stderr)
			}

			stubDoctorEnv(t)
			out, err := runDoctorIn(t, dir)
			if code := ExitCode(err); code != exitError {
				t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
			}
			assertLines(t, out, `^PASS +git `, `^FAIL +vibe\.yaml +`+regexp.QuoteMeta(msg)+`$`)
		})
	}
}

// guardSettings is a .claude/settings.json whose PreToolUse hook runs
// vibe-guard, as the setup skill's reference does.
const guardSettings = `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "vibe-guard hook"}]}],
 "Stop": [{"hooks": [{"type": "command", "command": "task verify:fast 1>&2 || exit 2"}]}]}}
`

// TestDoctorChecksTheWorkflow: on a machine with everything, doctor
// reports git, the configuration, each entrypoint program, bash for the
// hooks and vibe-guard, one line each, and exits 0.
func TestDoctorChecksTheWorkflow(t *testing.T) {
	dir := contractFixture(t, withFile(".claude/settings.json", guardSettings))
	stubDoctorEnv(t)
	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	assertLines(t, out,
		`^PASS +git `,
		`^PASS +vibe\.yaml +version 2, contract 1\.1\.0`,
		`^PASS +task +/stub/task; runs verify, verify-fast, fmt, fmt-check$`,
		`^PASS +bash +/stub/bash`,
		`^PASS +vibe-guard +/stub/vibe-guard`,
	)
}

// TestDoctorReportsWhatIsMissing: a missing entrypoint program, a missing
// bash and a missing vibe-guard each fail their own line, and doctor
// exits 1.
func TestDoctorReportsWhatIsMissing(t *testing.T) {
	for _, c := range []struct {
		missing string
		line    string
	}{
		{"task", `^FAIL +task +not on PATH; verify, verify-fast, fmt, fmt-check cannot run$`},
		{"bash", `^FAIL +bash +no bash found`},
		{"vibe-guard", `^FAIL +vibe-guard +not on PATH`},
	} {
		t.Run(c.missing, func(t *testing.T) {
			dir := contractFixture(t, withFile(".claude/settings.json", guardSettings))
			stubDoctorEnv(t, c.missing)
			out, err := runDoctorIn(t, dir)
			if code := ExitCode(err); code != exitError {
				t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
			}
			assertLines(t, out, c.line)
		})
	}
}

// TestDoctorChecksVibeGuardOnlyWhenAHookCallsIt: the baseline's guard
// hook is a task, so vibe-guard is not checked.
func TestDoctorChecksVibeGuardOnlyWhenAHookCallsIt(t *testing.T) {
	dir := contractFixture(t)
	stubDoctorEnv(t, "vibe-guard")
	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "vibe-guard") {
		t.Errorf("doctor checked vibe-guard, which no hook calls:\n%s", out)
	}
}

// TestDoctorEntrypointPrograms: each distinct program gets one line naming
// its entrypoints; a program given by path is looked for in the
// repository; a command that is not one simple command is left to V-04.
func TestDoctorEntrypointPrograms(t *testing.T) {
	dir := contractFixture(t,
		withFile("vibe.yaml", "contract: 1.1.0\nplatforms: [linux]\nagents: [claude]\nentrypoints:\n"+
			"  verify: [task verify, ./scripts/check.sh]\n  verify-fast: make fast\n  fmt: task fmt && task lint\n  fmt-check: ./scripts/gone.sh\n"),
		withFile("scripts/check.sh", "#!/bin/sh\n"))
	stubDoctorEnv(t, "make")
	out, _ := runDoctorIn(t, dir)
	assertLines(t, out,
		`^PASS +task +/stub/task; runs verify$`,
		`^PASS +\./scripts/check\.sh +in the repository; runs verify$`,
		`^FAIL +make +not on PATH; verify-fast cannot run$`,
		`^UNVERIFIED +entrypoints\.fmt +"task fmt && task lint" is a list joined by &&, not one simple command`,
		`^FAIL +\./scripts/gone\.sh +not in the repository; fmt-check cannot run$`,
	)
}

// TestDoctorAfterInit: in a repository vibe init just configured, with no
// hooks yet, doctor says the entrypoints are V-04's to declare and that
// nothing needs bash, and fails nothing.
func TestDoctorAfterInit(t *testing.T) {
	dir := contractFixture(t, withoutFile("vibe.yaml"), withoutFile(".claude/settings.json"))
	if out, err := runInit(t, dir); err != nil {
		t.Fatalf("vibe init: %v\n%s", err, out)
	}
	stubDoctorEnv(t, "bash")
	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	assertLines(t, out,
		`^UNVERIFIED +entrypoints +none declared; vibe audit's V-04 says what to declare$`,
		`^PASS +bash +not needed: \.claude/settings\.json has no command hook$`,
	)
}

// TestDoctorWritesNothing: doctor diagnoses and never mutates.
func TestDoctorWritesNothing(t *testing.T) {
	dir := contractFixture(t)
	before := snapshot(t, dir)

	stubDoctorEnv(t)
	if out, err := runDoctorIn(t, dir); err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if after := snapshot(t, dir); !maps.Equal(before, after) {
		t.Errorf("doctor changed the repository:\nbefore %v\nafter  %v", slices.Sorted(maps.Keys(before)), slices.Sorted(maps.Keys(after)))
	}
}

// snapshot maps every file under dir to its content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- walking the test's own temp directory
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
