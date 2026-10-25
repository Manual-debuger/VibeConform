package cli

import (
	"bytes"
	"context"
	"io"
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
// except those named missing, and every command succeeds. Files are the
// real ones, since the repository under test is the test's own temp
// directory.
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
			Run: func(_ context.Context, _, _ string, args ...string) (string, string, error) {
				return "v1\n", "", nil
			},
			Exists: func(path string) bool {
				_, err := os.Stat(path)
				return err == nil
			},
			Open: func(path string) (io.ReadCloser, error) {
				return os.Open(path) // #nosec G304 -- the test's own temp directory
			},
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

// Until vibe doctor reads the contract configuration (spec 0059, Desired
// Behavior 11), it checks the configuration the way vibe audit does: a
// version 1 file and a missing file fail with the audit's messages, and
// the checks that need no configuration still run.
func TestDoctorConfigurationLikeAudit(t *testing.T) {
	for _, c := range []struct {
		name, vibeYAML string
		want           []string
	}{
		{"no file", "", []string{"run `vibe init`"}},
		{"version 1", "standard: prod-go\nversion: v1\n", []string{"version 1", "vibe init --skill", migrationGuide}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if c.vibeYAML != "" {
				writeVibeYAML(t, dir, c.vibeYAML)
			}
			stubDoctorEnv(t)
			out, err := runDoctorIn(t, dir)
			if code := ExitCode(err); code != exitError {
				t.Fatalf("exit %d, want %d:\n%s", code, exitError, out)
			}
			for _, want := range []string{`(?m)^PASS +git `, `(?m)^FAIL +vibe\.yaml `, `(?m)^PASS +runtime `, `(?m)^PASS +worktree `} {
				if !regexp.MustCompile(want).MatchString(out) {
					t.Errorf("output does not match %s:\n%s", want, out)
				}
			}
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}
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
