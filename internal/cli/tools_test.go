package cli

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
	"github.com/Manual-debuger/VibeConform/internal/standard"
)

// stubLookPath makes the named binaries the only ones on PATH for the
// duration of one test, so the assertions are about this code rather than
// about what happens to be installed on the machine running it.
func stubLookPath(t *testing.T, installed ...string) {
	t.Helper()
	present := make(map[string]bool, len(installed))
	for _, name := range installed {
		present[name] = true
	}

	previous := lookPath
	lookPath = func(name string) (string, error) {
		if present[name] {
			return "/stub/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPath = previous })
}

// fakeModule is a module that resolves nothing and requires whatever the
// test says it requires.
type fakeModule struct {
	name  string
	tools []module.Tool
}

func (m fakeModule) Name() string { return m.name }

func (m fakeModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return nil, nil
}

func (m fakeModule) RequiredTools() []module.Tool { return m.tools }

// quietModule implements Module but not ToolRequirer, which is the majority
// case and must not need a nil-returning method to work.
type quietModule struct{}

func (quietModule) Name() string { return "quiet" }

func (quietModule) Resolve(_ context.Context, _ *module.Context) ([]resource.Resource, error) {
	return nil, nil
}

func TestWarnMissingToolsNamesToolModuleAndReason(t *testing.T) {
	stubLookPath(t /* nothing installed */)
	s := &standard.Standard{Modules: []module.Module{
		fakeModule{name: "go-tooling", tools: []module.Tool{{Name: "golangci-lint", Why: "task lint"}}},
	}}

	var buf bytes.Buffer
	warnMissingTools(&buf, s)

	want := "warning: golangci-lint not found on PATH (required by go-tooling: task lint)\n"
	if got := buf.String(); got != want {
		t.Errorf("warning = %q, want %q", got, want)
	}
}

func TestWarnMissingToolsSaysNothingWhenEverythingIsPresent(t *testing.T) {
	stubLookPath(t, "task", "lefthook")
	s := &standard.Standard{Modules: []module.Module{
		fakeModule{name: "repo-tooling", tools: []module.Tool{{Name: "task"}, {Name: "lefthook"}}},
		quietModule{},
	}}

	var buf bytes.Buffer
	warnMissingTools(&buf, s)

	if buf.Len() != 0 {
		t.Errorf("a fully equipped machine should produce no warnings, got:\n%s", buf.String())
	}
}

// TestMissingToolsReportsEachBinaryOnce pins both halves of the ordering
// contract: module then declaration order, and one line per missing binary
// however many modules asked for it.
func TestMissingToolsReportsEachBinaryOnce(t *testing.T) {
	stubLookPath(t /* nothing installed */)
	s := &standard.Standard{Modules: []module.Module{
		fakeModule{name: "first", tools: []module.Tool{{Name: "alpha"}, {Name: "shared"}}},
		quietModule{},
		fakeModule{name: "second", tools: []module.Tool{{Name: "shared"}, {Name: "beta"}}},
	}}

	got := missingTools(s)

	want := []missingTool{
		{module: "first", name: "alpha"},
		{module: "first", name: "shared"},
		{module: "second", name: "beta"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d missing tools, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].name != want[i].name || got[i].module != want[i].module {
			t.Errorf("missing[%d] = %s (from %s), want %s (from %s)",
				i, got[i].name, got[i].module, want[i].name, want[i].module)
		}
	}
}

// TestSyncWarnsWithoutChangingExitCode is the invariant the whole increment
// rests on. A missing tool is not non-conformance: the files are correct,
// and audit will say so on the same repository.
func TestSyncWarnsWithoutChangingExitCode(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	stubHookInstall(t)
	stubLookPath(t /* nothing installed */)

	out, errOut, err := runSyncCapturing(t, dir)
	if err != nil {
		t.Fatalf("missing tools must not fail a sync: %v\n%s", err, out)
	}

	for _, want := range []string{
		"warning: golangci-lint not found on PATH (required by go-tooling",
		"warning: task not found on PATH (required by repo-tooling",
		"warning: lefthook not found on PATH (required by repo-tooling",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q\n%s", want, errOut)
		}
	}
	if strings.Contains(out, "warning") {
		t.Errorf("warnings belong on stderr, not in sync's report of what it did\n%s", out)
	}

	// The same repository, audited with the same empty PATH, is conformant.
	auditOut, auditErr := runAuditIn(t, dir)
	if auditErr != nil {
		t.Fatalf("audit disagreed with sync about a repository whose files are correct: %v\n%s", auditErr, auditOut)
	}
	if !strings.Contains(auditOut, "conformant") || strings.Contains(auditOut, "not conformant") {
		t.Errorf("audit should report conformant regardless of tool availability\n%s", auditOut)
	}
}

// TestSyncReportsMissingLefthookOnlyOnce covers the seam between this
// increment and the last: registerGitHooks stays silent when the binary is
// absent, because the warning above the report already said so.
func TestSyncReportsMissingLefthookOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir)
	stubLookPath(t /* nothing installed */)
	// The real implementation, not the seam: it must find no lefthook and
	// report that as errLefthookNotFound without printing anything itself.
	t.Setenv("PATH", "")

	_, errOut, err := runSyncCapturing(t, dir)
	if err != nil {
		t.Fatalf("sync returned error: %v", err)
	}

	// Lines, not substring occurrences: repo-tooling's reason text names
	// lefthook.yml, so one warning mentions the word twice.
	var mentions int
	for line := range strings.SplitSeq(strings.TrimSpace(errOut), "\n") {
		if strings.Contains(line, "lefthook") {
			mentions++
		}
	}
	if mentions != 1 {
		t.Errorf("lefthook named on %d stderr lines, want exactly 1:\n%s", mentions, errOut)
	}
	if strings.Contains(errOut, "git hooks were not registered") {
		t.Errorf("hook registration should stay silent when the binary is already reported missing:\n%s", errOut)
	}
}

// TestRegisteredModulesDeclareTheirTools guards the declarations themselves,
// which are the part a future module is most likely to forget.
func TestRegisteredModulesDeclareTheirTools(t *testing.T) {
	s, err := standard.Lookup("prod-go", "v1")
	if err != nil {
		t.Fatalf("looking up prod-go/v1: %v", err)
	}

	declared := map[string]string{}
	for _, mod := range s.Modules {
		requirer, ok := mod.(module.ToolRequirer)
		if !ok {
			continue
		}
		for _, tool := range requirer.RequiredTools() {
			if tool.Why == "" {
				t.Errorf("%s requires %s without saying what it is for", mod.Name(), tool.Name)
			}
			declared[tool.Name] = mod.Name()
		}
	}

	for tool, wantModule := range map[string]string{
		"golangci-lint": "go-tooling",
		"task":          "repo-tooling",
		"lefthook":      "repo-tooling",
	} {
		if got := declared[tool]; got != wantModule {
			t.Errorf("%s declared by %q, want %q", tool, got, wantModule)
		}
	}
}
