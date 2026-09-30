package cli

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/doctor"
)

// stubDoctorEnv makes doctor see a machine where every binary is on PATH
// except those named missing, and every command succeeds and prints v1.
func stubDoctorEnv(t *testing.T, missing ...string) {
	t.Helper()
	previous := doctorEnv
	doctorEnv = func() doctor.Env {
		return doctor.Env{
			LookPath: func(name string) (string, error) {
				for _, m := range missing {
					if m == name {
						return "", exec.ErrNotFound
					}
				}
				return "/stub/" + name, nil
			},
			Run: func(context.Context, string, string, ...string) (string, string, error) {
				return "v1\n", "", nil
			},
			Exists: func(string) bool { return false },
			GOOS:   "linux",
			GOARCH: "amd64",
		}
	}
	t.Cleanup(func() { doctorEnv = previous })
}

func runDoctorIn(t *testing.T, dir string) (string, error) {
	t.Helper()
	root := NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doctor", "--repo-root", dir})
	err := root.Execute()
	return out.String(), err
}

func TestDoctorHealthyExitsZero(t *testing.T) {
	stubDoctorEnv(t)
	dir := t.TempDir()
	writeManifest(t, dir)

	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	for _, want := range []string{
		"standard: prod-go/v1\n",
		"PASS        git",
		"vibe.yaml resolves prod-go/v1 (integrations: claude, codex)",
		"; Taskfile.yml loads",
		"summary: ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("a healthy machine reports a failure:\n%s", out)
	}
}

func TestDoctorMissingToolExitsOne(t *testing.T) {
	stubDoctorEnv(t, "golangci-lint")
	dir := t.TempDir()
	writeManifest(t, dir)

	out, err := runDoctorIn(t, dir)
	if err == nil {
		t.Fatalf("doctor passed with golangci-lint missing:\n%s", out)
	}
	if code := ExitCode(err); code != exitError {
		t.Errorf("exit %d, want %d: a required FAIL is 'no usable verdict', not a conformance code", code, exitError)
	}
	if !strings.Contains(err.Error(), "doctor: 1 required check failed") {
		t.Errorf("error = %q", err)
	}
	if !strings.Contains(out, "FAIL        golangci-lint  not found on PATH (required by go-tooling: ") {
		t.Errorf("output does not name the missing tool and who needs it:\n%s", out)
	}
}

// TestDoctorWithoutManifest: no vibe.yaml fails the manifest check, but
// the checks that don't need it still run.
func TestDoctorWithoutManifest(t *testing.T) {
	stubDoctorEnv(t)
	out, err := runDoctorIn(t, t.TempDir())
	if ExitCode(err) != exitError {
		t.Fatalf("exit %d, want %d:\n%s", ExitCode(err), exitError, out)
	}
	for _, want := range []string{"PASS        git", "FAIL        manifest", "UNVERIFIED  tools     needs a valid vibe.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "standard:") {
		t.Errorf("no standard resolved, but the output names one:\n%s", out)
	}
}
