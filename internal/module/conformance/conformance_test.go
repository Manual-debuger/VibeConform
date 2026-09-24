package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "vibe-conformance" {
		t.Errorf("Name() = %q, want %q", got, "vibe-conformance")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{".github/workflows/conformance.yml", "Taskfile.vibe.yml"}
	if len(resources) != len(want) {
		t.Fatalf("got %d resources, want %d", len(resources), len(want))
	}
	for i, r := range resources {
		if r.Path != want[i] {
			t.Errorf("resource %d path = %q, want %q", i, r.Path, want[i])
		}
		if r.Ownership != resource.Generated {
			t.Errorf("%s ownership = %v, want Generated", r.Path, r.Ownership)
		}
		if len(r.Content) == 0 {
			t.Errorf("%s has empty content", r.Path)
		}
	}
}

func TestResolveDeterministic(t *testing.T) {
	first, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	second, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for i := range first {
		if first[i].Path != second[i].Path || !bytes.Equal(first[i].Content, second[i].Content) {
			t.Errorf("resource %d differs across calls", i)
		}
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for path, content := range map[string][]byte{
		"conformance.yml":   conformanceWorkflow,
		"Taskfile.vibe.yml": vibeTaskfile,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes", path)
		}
	}
}

// TestTemplatesMatchLiveFiles is the drift alarm between this repository's
// own synced copies and the templates they were synced from.
func TestTemplatesMatchLiveFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	for live, embedded := range map[string][]byte{
		filepath.Join(".github", "workflows", "conformance.yml"): conformanceWorkflow,
		"Taskfile.vibe.yml": vibeTaskfile,
	} {
		got, err := os.ReadFile(filepath.Join(repoRoot, live))
		if err != nil {
			t.Fatalf("reading live file: %v", err)
		}
		if !bytes.Equal(got, embedded) {
			t.Errorf("embedded template has drifted from %s", live)
		}
	}
}

// TestWorkflowIsPinnedAndSelfContained pins spec 0022's workflow: one job,
// named as the required check, that runs task audit and never installs a
// floating vibe. Pinning is task audit's job, so there is no vibe install
// step for a later edit to point back at @latest.
func TestWorkflowIsPinnedAndSelfContained(t *testing.T) {
	text := string(conformanceWorkflow)
	if strings.Contains(text, "@latest") {
		t.Error("conformance.yml installs something @latest; its verdict would change with no change to the repository")
	}
	if regexp.MustCompile(`go install \S*cmd/vibe`).MatchString(text) {
		t.Error("conformance.yml installs vibe itself; task audit's pinned fallback is the only way it may obtain vibe")
	}
	if !strings.Contains(text, "if [ -d ./cmd/vibe ]") {
		t.Error("self-hosting probe missing (ADR 0008)")
	}

	var wf struct {
		Jobs map[string]struct {
			Name  string `yaml:"name"`
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(conformanceWorkflow, &wf); err != nil {
		t.Fatalf("parsing conformance.yml: %v", err)
	}
	if len(wf.Jobs) != 1 {
		t.Fatalf("conformance.yml has %d jobs, want 1", len(wf.Jobs))
	}
	job, ok := wf.Jobs["audit"]
	if !ok {
		t.Fatal("conformance.yml has no audit job")
	}
	if job.Name != "Conformance / audit" {
		t.Errorf("audit job name = %q, want %q: branch protection requires that exact check name",
			job.Name, "Conformance / audit")
	}
	if n := len(job.Steps); n == 0 || strings.TrimSpace(job.Steps[n-1].Run) != "task audit" {
		t.Error("the audit job's last step is not `task audit`")
	}
}

// auditScript returns task audit's single shell command.
func auditScript(t *testing.T) string {
	t.Helper()
	var tf struct {
		Tasks map[string]struct {
			Cmds []string `yaml:"cmds"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(vibeTaskfile, &tf); err != nil {
		t.Fatalf("parsing Taskfile.vibe.yml: %v", err)
	}
	if len(tf.Tasks) != 1 {
		t.Errorf("Taskfile.vibe.yml defines %d tasks, want only audit", len(tf.Tasks))
	}
	audit, ok := tf.Tasks["audit"]
	if !ok || len(audit.Cmds) != 1 {
		t.Fatal("Taskfile.vibe.yml must define audit with exactly one command")
	}
	return audit.Cmds[0]
}

// TestAuditReachesVibeOnlyFromPathOrPinnedModule carries specs 0017 and
// 0018's audit invariants over from the repo-tooling Taskfiles. audit must
// reach vibe, and only as a PATH-resolved binary or the published module at
// the recorded version — never built out of the repository under audit,
// which in an adopting repository has no cmd/vibe (issue #20).
//
// Stated positively: every reference to cmd/vibe must be the published
// module path, so a new way of smuggling in a local path fails without
// anyone having to think of it first.
func TestAuditReachesVibeOnlyFromPathOrPinnedModule(t *testing.T) {
	script := auditScript(t)

	fromPath := false
	for line := range strings.Lines(script) {
		fromPath = fromPath || strings.TrimSpace(line) == "vibe audit --repo-root ."
	}
	if !fromPath {
		t.Error("task audit never runs a PATH-resolved `vibe audit --repo-root .`")
	}

	allowed := map[string]bool{
		// The fallback itself.
		`github.com/Manual-debuger/VibeConform/cmd/vibe@"$v"`: true,
		// The install hint in the failure message.
		"github.com/Manual-debuger/VibeConform/cmd/vibe@<version>)": true,
	}
	for _, ref := range regexp.MustCompile(`\S*cmd/vibe\S*`).FindAllString(script, -1) {
		if !allowed[ref] {
			t.Errorf("task audit refers to %q; vibe may only come from PATH or the published "+
				"module at the recorded version (specs 0018, 0022)", ref)
		}
	}
	if strings.Contains(script, "@latest") {
		t.Error("task audit fetches vibe @latest; it must use the recorded vibe_version")
	}
}

type pinnableCase struct {
	Version  string `json:"version"`
	Pinnable bool   `json:"pinnable"`
	Why      string `json:"why"`
}

// TestAuditFallbackNeverPassesSilently runs task audit for real, with
// neither vibe nor go on PATH, across the shared table of recorded versions
// internal/state's Pinnable test also reads — so the shell's idea of a
// fetchable version and vibe sync's cannot drift apart (spec 0022, 3a).
//
// A pinnable version gets as far as needing go and says so; anything else
// is refused as unfetchable. Every case must exit non-zero: there is no
// path to exit 0 without vibe audit having run.
func TestAuditFallbackNeverPassesSilently(t *testing.T) {
	taskBin, err := exec.LookPath("task")
	if err != nil {
		t.Skip("task not on PATH")
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "state", "testdata", "pinnable_versions.json"))
	if err != nil {
		t.Fatalf("reading shared version table: %v", err)
	}
	var cases []pinnableCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parsing shared version table: %v", err)
	}

	type run struct {
		name  string
		state *string // nil: no state file at all
		crlf  bool
		want  string
	}
	var runs []run
	for _, c := range cases {
		want := "cannot be fetched"
		switch {
		case c.Pinnable:
			want = "go is not on PATH"
		case c.Version == "":
			want = "records no vibe_version"
		}
		body := "schema: 2\nstandard: prod-go/v1\n"
		if c.Version != "" {
			body = "schema: 2\nvibe_version: " + c.Version + "\nstandard: prod-go/v1\n"
		}
		runs = append(runs, run{name: c.Why + " " + c.Version, state: &body, want: want})
		if c.Pinnable {
			runs = append(runs, run{name: c.Why + " " + c.Version + " CRLF", state: &body, crlf: true, want: want})
		}
	}
	runs = append(runs, run{name: "no state file", want: "records no vibe_version"})

	for _, r := range runs {
		t.Run(r.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), vibeTaskfile, 0o644); err != nil {
				t.Fatal(err)
			}
			if r.state != nil {
				body := *r.state
				if r.crlf {
					body = strings.ReplaceAll(body, "\n", "\r\n")
				}
				if err := os.MkdirAll(filepath.Join(dir, ".vibe"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, ".vibe", "state.yaml"), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			cmd := exec.Command(taskBin, "audit")
			cmd.Dir = dir
			cmd.Env = envWithPath(t.TempDir())
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("task audit exited 0 with neither vibe nor go on PATH:\n%s", out)
			}
			if !strings.Contains(string(out), r.want) {
				t.Errorf("output does not say %q:\n%s", r.want, out)
			}
		})
	}
}

// envWithPath is the test process's environment with PATH replaced by dir
// alone. Windows spells the variable Path and matches it case-insensitively.
func envWithPath(dir string) []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == "PATH" || (runtime.GOOS == "windows" && strings.EqualFold(name, "PATH")) {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "PATH="+dir)
}
