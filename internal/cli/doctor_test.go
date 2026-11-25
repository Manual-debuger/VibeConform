package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// The checks doctor runs, and their order, are internal/doctor's tests
// (doctor.Run, spec 0065). The CLI's own are that vibe doctor prints the
// report and sets the exit code, here and in config_messages_test.go.

// stubDoctorEnv is a machine where every binary is on PATH except those
// named missing, every command succeeds, and bash resolves unless "bash"
// is missing. Files are the real ones, since the repository under test
// is the test's own temp directory.
func stubDoctorEnv(missing ...string) func() doctor.Env {
	return func() doctor.Env {
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
}

// runDoctorIn runs vibe doctor on dir, on a stub machine where the
// programs named missing are not found.
func runDoctorIn(t *testing.T, dir string, missing ...string) (string, error) {
	t.Helper()
	root := newRootCmd("test", contractSeams{openRepo: contracttest.OpenRepo, doctorEnv: stubDoctorEnv(missing...)})
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

// guardSettings is a .claude/settings.json whose PreToolUse hook runs
// vibe-guard, as the setup skill's reference does.
const guardSettings = `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "vibe-guard hook"}]}],
 "Stop": [{"hooks": [{"type": "command", "command": "task verify:fast 1>&2 || exit 2"}]}]}}
`

// TestDoctorChecksTheWorkflow: vibe doctor prints doctor.Run's report,
// one aligned line per check and the summary, and exits 0 when nothing
// fails; one failed check exits 1.
func TestDoctorChecksTheWorkflow(t *testing.T) {
	t.Parallel()
	t.Run("nothing fails", func(t *testing.T) {
		t.Parallel()
		dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile(".claude/settings.json", guardSettings))
		out, err := runDoctorIn(t, dir)
		if err != nil {
			t.Fatalf("doctor: %v\n%s", err, out)
		}
		want := "" +
			"PASS        git         v1 (/stub/git); repository readable\n" +
			"PASS        vibe.yaml   version 2, contract 1.2.0\n" +
			"PASS        task        /stub/task; runs verify, verify-fast, fmt, fmt-check\n" +
			"PASS        bash        /stub/bash; runs the hook commands\n" +
			"PASS        vibe-guard  /stub/vibe-guard; the PreToolUse hook runs it\n" +
			"summary: 5 pass, 0 fail, 0 unverified\n"
		if out != want {
			t.Errorf("stdout:\n got %q\nwant %q", out, want)
		}
	})
	t.Run("a check fails", func(t *testing.T) {
		t.Parallel()
		dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile(".claude/settings.json", guardSettings))
		out, err := runDoctorIn(t, dir, "task")
		if code := ExitCode(err); code != exitError {
			t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
		}
		if want := "doctor: 1 check failed"; err.Error() != want {
			t.Errorf("error %q, want %q", err, want)
		}
		assertLines(t, out, `^FAIL +task +not on PATH; verify, verify-fast, fmt, fmt-check cannot run$`)
	})
}
