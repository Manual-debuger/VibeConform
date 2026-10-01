package cli

import (
	"bytes"
	"context"
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
	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/standard"
)

// stubDoctorEnv makes doctor see a machine where every binary is on PATH
// except those named missing, and every command succeeds: version probes
// print v1, and git reports eol=lf for any path. Files are the real ones,
// since the repository under test is the test's own temp directory.
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
				if len(args) > 0 && args[0] == "check-attr" {
					return args[len(args)-1] + ": eol: lf\n", "", nil
				}
				return "v1\n", "", nil
			},
			Exists: func(path string) bool {
				_, err := os.Stat(path)
				return err == nil
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

// syncedRepo returns a temp directory holding a synced prod-go
// repository, so the files doctor checks for are there.
func syncedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeManifest(t, dir)
	if out, err := runSyncIn(t, dir); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	return dir
}

func TestDoctorHealthyExitsZero(t *testing.T) {
	dir := syncedRepo(t)
	stubDoctorEnv(t)

	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	for _, want := range []string{
		"standard: prod-go/v1\n",
		"PASS        git",
		"vibe.yaml resolves prod-go/v1 (integrations: claude, codex)",
		"; Taskfile.yml loads",
		"PASS        claude hooks",
		"; .golangci.yml has eol=lf; policy.line_endings is not selected",
		"summary: 14 pass, 0 warn, 0 fail, 0 unverified",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("a healthy machine reports a failure:\n%s", out)
	}
}

// TestDoctorReportsLineEndingPolicy: with the policy selected, the line
// endings row reports its health instead of the bare attribute.
func TestDoctorReportsLineEndingPolicy(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goLF)
	mustSync(t, dir)
	stubDoctorEnv(t)

	out, err := runDoctorIn(t, dir)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if want := "PASS        line endings   policy line_endings: lf; core.autocrlf="; !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
}

func TestDoctorMissingToolExitsOne(t *testing.T) {
	dir := syncedRepo(t)
	stubDoctorEnv(t, "golangci-lint")

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
	for _, want := range []string{
		`(?m)^PASS +git `,
		`(?m)^FAIL +manifest `,
		`(?m)^UNVERIFIED +tools +needs a valid vibe\.yaml$`,
		`(?m)^UNVERIFIED +agent hooks +needs a valid vibe\.yaml$`,
		`(?m)^PASS +runtime `,
		`(?m)^PASS +worktree `,
	} {
		if !regexp.MustCompile(want).MatchString(out) {
			t.Errorf("output does not match %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "standard:") {
		t.Errorf("no standard resolved, but the output names one:\n%s", out)
	}
}

// TestAgentHookConfigsCoverCatalog: every agent a standard's catalog
// offers has a doctor row, so a new agent cannot ship without doctor
// saying whether its hooks can run.
func TestAgentHookConfigsCoverCatalog(t *testing.T) {
	for _, name := range []string{"prod-go", "prod-ts", "prod-py", "prod-mono"} {
		s, err := standard.Lookup(name, "v1")
		if err != nil {
			t.Fatal(err)
		}
		for _, in := range s.Options {
			if in.Group.Key != manifest.CategoryAgents {
				continue
			}
			if _, ok := agentHookConfigs[in.Name]; !ok {
				t.Errorf("%s offers agent %q, which has no agentHookConfigs row", name, in.Name)
			}
		}
	}
}

// TestDoctorHookBinaries: the claude line names task and the runtime the
// standard's guard starts, taken from each committed repository.
func TestDoctorHookBinaries(t *testing.T) {
	for root, want := range map[string][]string{
		filepath.Join("..", ".."):                           {"task", "go"},
		filepath.Join("..", "..", "examples", "typescript"): {"task", "node"},
		filepath.Join("..", "..", "examples", "python"):     {"task", "uv"},
		// Go, TypeScript and Python components: the guard runs on Go.
		filepath.Join("..", "..", "examples", "monorepo"): {"task", "go"},
	} {
		p, err := buildPlan(root)
		if err != nil {
			t.Fatal(err)
		}
		hooks := agentHooks(p)
		if len(hooks) == 0 || hooks[0].Name != "claude" {
			t.Fatalf("%s: agent hooks = %+v, want claude first", root, hooks)
		}
		if !slices.Equal(hooks[0].Binaries, want) {
			t.Errorf("%s: claude hook binaries = %v, want %v", root, hooks[0].Binaries, want)
		}
	}
}

// TestDoctorWritesNothing: doctor diagnoses and never mutates. A synced
// repository is byte-for-byte the same after it runs, state included.
func TestDoctorWritesNothing(t *testing.T) {
	dir := syncedRepo(t)
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
