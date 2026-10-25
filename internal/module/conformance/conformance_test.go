package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestMain(m *testing.M) {
	// A git hook running the suite, such as pre-push in a linked worktree,
	// exports GIT_DIR. Every git the tests start would then use this
	// repository instead of the one in its temp directory.
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		_ = os.Unsetenv(v)
	}
	os.Exit(m.Run())
}

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

// TestWorkflowIsPinnedAndSelfContained pins spec 0022's workflow as spec
// 0054 changed it: one job, named as the required check, that installs
// exactly the vibe_version .vibe/state.yaml records through the setup
// action, pinned by commit SHA, and then runs vibe audit itself. It needs
// no Go and no Task: setup-go is gone, and the action's source auto builds
// from the checkout only in VibeConform itself (ADR 0008).
func TestWorkflowIsPinnedAndSelfContained(t *testing.T) {
	text := string(conformanceWorkflow)
	if strings.Contains(text, "@latest") {
		t.Error("conformance.yml installs something @latest; its verdict would change with no change to the repository")
	}
	if regexp.MustCompile(`go install \S*cmd/vibe`).MatchString(text) {
		t.Error("conformance.yml installs vibe itself; the setup action is the only way it may obtain vibe")
	}
	for _, gone := range []string{"setup-go", "go install", "go build", "task audit", "go-task"} {
		if strings.Contains(text, gone) {
			t.Errorf("conformance.yml still names %q; the job needs neither Go nor Task (spec 0054)", gone)
		}
	}

	var wf struct {
		Jobs map[string]struct {
			Name  string `yaml:"name"`
			Steps []struct {
				ID   string            `yaml:"id"`
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				With map[string]string `yaml:"with"`
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
	if n := len(job.Steps); n == 0 || strings.TrimSpace(job.Steps[n-1].Run) != "vibe audit --repo-root ." {
		t.Error("the audit job's last step is not `vibe audit --repo-root .`")
	}

	var setup int
	for i, s := range job.Steps {
		if strings.HasPrefix(s.Uses, "Manual-debuger/VibeConform") {
			setup = i
			if !regexp.MustCompile(`^Manual-debuger/VibeConform@[0-9a-f]{40}$`).MatchString(s.Uses) {
				t.Errorf("the setup action is %q, want it pinned by a full commit SHA (C-02)", s.Uses)
			}
			if !strings.Contains(s.With["version"], "steps.") || !strings.Contains(s.With["version"], ".outputs.") {
				t.Errorf("the setup action's version is %q, want the vibe_version read from .vibe/state.yaml", s.With["version"])
			}
			if src, ok := s.With["source"]; ok && src != "auto" {
				t.Errorf("the setup action's source is %q, want auto", src)
			}
		}
	}
	if setup == 0 {
		t.Fatal("conformance.yml does not use the setup action after checkout")
	}
	read := job.Steps[setup-1]
	if !strings.Contains(read.Run, ".vibe/state.yaml") || !strings.Contains(read.Run, "vibe_version") {
		t.Errorf("the step before the setup action does not read vibe_version from .vibe/state.yaml:\n%s", read.Run)
	}
	if !strings.Contains(job.Steps[setup].With["version"], "steps."+read.ID+".outputs.") {
		t.Errorf("the setup action's version does not come from step %q", read.ID)
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
// which in an adopting repository has no cmd/vibe (issue #20). The one
// exception is VibeConform's own checkout (spec 0057, ADR 0008): the probe
// for cmd/vibe and the build of ./cmd/vibe, which
// TestAuditSelfBuildOnlyInVibeConform shows run only there.
//
// Stated positively: every reference to cmd/vibe must be the published
// module path or that branch's, so a new way of smuggling in a local path
// fails without anyone having to think of it first.
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
		// VibeConform's own checkout: the probe and the build (spec 0057).
		"cmd/vibe":   true,
		"./cmd/vibe": true,
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

// TestAuditSelfBuildOnlyInVibeConform pins spec 0057 Desired Behavior 12
// by running task audit: in a checkout whose go.mod declares VibeConform's
// module and that has cmd/vibe, audit builds that vibe and runs it ahead
// of a vibe on PATH, and its exit code (2, not conformant) reaches the
// caller. Another module path, or no cmd/vibe, runs the vibe on PATH.
func TestAuditSelfBuildOnlyInVibeConform(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which builds Go programs; task verify runs it")
	}
	var dirs []string
	for _, tool := range []string{"task", "go", "git"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not on PATH", tool)
		}
		dirs = append(dirs, filepath.Dir(p))
	}

	// A vibe on PATH that only says it ran.
	fake := t.TempDir()
	writeFiles(t, fake, map[string]string{
		"go.mod":  "module example.com/fakevibe\n\ngo 1.27\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"PATH-VIBE-RAN\") }\n",
	})
	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "vibe"+exeSuffix()), ".")
	build.Dir = fake
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the fake vibe: %v\n%s", err, out)
	}
	path := strings.Join(append([]string{bin}, dirs...), string(os.PathListSeparator))

	// The checkout's vibe prints its arguments and exits 2.
	selfMain := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"strings\"\n)\n\n" +
		"func main() {\n\tfmt.Println(\"self-built vibe \" + strings.Join(os.Args[1:], \" \"))\n\tos.Exit(2)\n}\n"
	for _, tc := range []struct {
		name   string
		gomod  string
		cmd    bool
		self   bool
		wantRC int
	}{
		{"VibeConform", "module github.com/Manual-debuger/VibeConform\n\ngo 1.27\n", true, true, 2},
		{"VibeConform CRLF", "module github.com/Manual-debuger/VibeConform \r\n\r\ngo 1.27\r\n", true, true, 2},
		{"another module", "module example.com/adopter\n\ngo 1.27\n", true, false, 0},
		{"a module path VibeConform's is a prefix of", "module github.com/Manual-debuger/VibeConformFork\n\ngo 1.27\n", true, false, 0},
		{"no cmd/vibe", "module github.com/Manual-debuger/VibeConform\n\ngo 1.27\n", false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			files := map[string]string{"Taskfile.yml": string(vibeTaskfile), "go.mod": tc.gomod}
			if tc.cmd {
				files["cmd/vibe/main.go"] = selfMain
			}
			writeFiles(t, dir, files)
			if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}

			cmd := exec.Command("task", "-x", "audit")
			cmd.Dir = dir
			cmd.Env = envWithPath(path)
			out, err := cmd.CombinedOutput()
			rc := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatalf("running task audit: %v", err)
				}
				rc = exit.ExitCode()
			}
			ranSelf := strings.Contains(string(out), "self-built vibe audit --repo-root .")
			ranPath := strings.Contains(string(out), "PATH-VIBE-RAN")
			if rc != tc.wantRC || ranSelf != tc.self || ranPath == tc.self {
				t.Errorf("exit %d, ran the checkout's vibe %v, the PATH vibe %v; want exit %d, checkout's %v\n%s",
					rc, ranSelf, ranPath, tc.wantRC, tc.self, out)
			}
		})
	}
}

// writeFiles writes each file, by slash path relative to dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// exeSuffix is ".exe" on Windows, where PATH finds programs by it.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
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
