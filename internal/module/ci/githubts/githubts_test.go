package githubts

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/resource"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "github-ci-ts" {
		t.Errorf("Name() = %q, want %q", got, "github-ci-ts")
	}
}

func TestResolveReturnsExpectedResourcesInOrder(t *testing.T) {
	resources, err := New().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := []string{
		".github/workflows/ci.yml",
		".github/dependabot.yml",
		".github/pull_request_template.md",
	}
	if len(resources) != len(want) {
		t.Fatalf("Resolve returned %d resources, want %d", len(resources), len(want))
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
		if strings.Contains(r.Path, "\\") {
			t.Errorf("path %q contains a backslash; paths must be slash-separated", r.Path)
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

// TestDependabotIsNPMNotGoMod guards the reason this package exists rather
// than reusing internal/module/ci/github's dependabot.yml as-is: that one
// hardcodes package-ecosystem: gomod, which is Go-specific despite the
// file's generic name.
func TestDependabotIsNPMNotGoMod(t *testing.T) {
	if !bytes.Contains(dependabotConfig, []byte("package-ecosystem: npm")) {
		t.Error("dependabot.yml does not declare the npm ecosystem")
	}
	if bytes.Contains(dependabotConfig, []byte("gomod")) {
		t.Error("dependabot.yml still names the Go ecosystem")
	}
}

// TestWorkflowUsesPnpm guards spec 0020's CI half: every Node job installs
// through pnpm with a frozen lockfile, sets pnpm up with a SHA-pinned action
// before Node (setup-node's pnpm cache needs pnpm on PATH), and no longer
// runs npm.
func TestWorkflowUsesPnpm(t *testing.T) {
	wf := string(ciWorkflow)
	if strings.Contains(wf, "npm ci") {
		t.Error("ci.yml still installs with npm ci")
	}

	nodeJobs := strings.Count(wf, "uses: actions/setup-node@")
	if nodeJobs == 0 {
		t.Fatal("ci.yml sets up Node in no job")
	}
	if got := strings.Count(wf, "pnpm install --frozen-lockfile"); got != nodeJobs {
		t.Errorf("ci.yml has %d frozen pnpm installs for %d Node jobs, want one each", got, nodeJobs)
	}
	if got := strings.Count(wf, "cache: pnpm"); got != nodeJobs {
		t.Errorf("ci.yml has %d setup-node steps with cache: pnpm, want %d", got, nodeJobs)
	}
	pin := regexp.MustCompile(`uses: pnpm/action-setup@[0-9a-f]{40} # v`)
	if got := len(pin.FindAllString(wf, -1)); got != nodeJobs {
		t.Errorf("ci.yml has %d SHA-pinned pnpm/action-setup steps, want %d", got, nodeJobs)
	}

	// Jobs are the two-space-indented keys under jobs:. Within each one,
	// pnpm must be set up before Node.
	for _, job := range regexp.MustCompile(`(?m)^  [a-z_]+:$`).Split(wf, -1)[1:] {
		pnpm := strings.Index(job, "pnpm/action-setup@")
		node := strings.Index(job, "actions/setup-node@")
		if node >= 0 && (pnpm < 0 || pnpm > node) {
			t.Errorf("a job sets up Node before pnpm:\n%s", job)
		}
	}
}

// TestTemplatesAreLF — see internal/module/ci/github for why CR bytes in an
// embedded template make the binary's output platform-dependent.
func TestTemplatesAreLF(t *testing.T) {
	for name, content := range map[string][]byte{
		"ci.yml":                   ciWorkflow,
		"dependabot.yml":           dependabotConfig,
		"pull_request_template.md": pullRequestTemplate,
	} {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF", name)
		}
	}
}
