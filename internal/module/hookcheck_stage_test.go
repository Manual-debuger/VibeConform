package module_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// hookCheckWant is, per Taskfile, the checks hook:check runs: the
// standard's typecheck and lint, and each monorepo component's for the
// monorepo.
var hookCheckWant = map[string][]string{
	"repotooling/templates/Taskfile.yml":          {"lint", "typecheck"},
	"tsrepotooling/templates/Taskfile.yml":        {"lint", "typecheck"},
	"pyrepotooling/templates/Taskfile.yml":        {"lint", "typecheck"},
	"monorepotooling/templates/Taskfile.yml.tmpl": {"api:lint", "api:typecheck", "web:lint", "web:typecheck", "worker:lint", "worker:typecheck"},
	"setupskill/reference/go":                     {"lint", "typecheck"},
	"setupskill/reference/typescript":             {"lint", "typecheck"},
	"setupskill/reference/python":                 {"lint", "typecheck"},
	"setupskill/reference/monorepo":               {"api:lint", "api:typecheck", "web:lint", "web:typecheck", "worker:lint", "worker:typecheck"},
}

// agentPayload is a PostToolUse payload for an edit of path with a
// top-level agent_id, as Claude Code sends it from inside a sub-agent.
func agentPayload(t *testing.T, path, agentID string, indent bool) string {
	t.Helper()
	payload := map[string]any{
		"session_id":      "s",
		"agent_id":        agentID,
		"agent_type":      "general-purpose",
		"hook_event_name": "PostToolUse",
		"tool_name":       "Edit",
		"tool_input":      map[string]any{"file_path": path, "old_string": "a", "new_string": "b"},
	}
	var data []byte
	var err error
	if indent {
		data, err = json.MarshalIndent(payload, "", "  ")
	} else {
		data, err = json.Marshal(payload)
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestHookCheckRunsTypecheckAndLintOnly pins spec 0057 Desired Behavior 1
// and 2 for every Taskfile that defines hook:check: it runs the standard's
// typecheck and lint and no tests (every test task is a stub that fails),
// and a payload from a sub-agent (a non-empty agent_id, in either JSON
// spacing) runs nothing, even where the path filter would run the checks.
func TestHookCheckRunsTypecheckAndLintOnly(t *testing.T) {
	task, err := exec.LookPath("task")
	if err != nil {
		if os.Getenv("VIBE_HOOKS_E2E") != "" {
			t.Fatalf("VIBE_HOOKS_E2E is set but task is not on PATH: %v", err)
		}
		t.Skip("task not on PATH; the hook-guard workflow runs this in CI")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	cases := []struct {
		name    string
		noGit   bool
		payload func(t *testing.T, repo string) string
		checks  bool
	}{
		{"main session", false, func(t *testing.T, repo string) string {
			return editPayload(t, "file_path", filepath.Join(repo, "a b.go"), false)
		}, true},
		{"sub-agent", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "a1b2c3", false)
		}, false},
		{"sub-agent, indented payload", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "a1b2c3", true)
		}, false},
		{"sub-agent, not a git repository", true, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "a1b2c3", false)
		}, false},
		{"empty agent_id", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "", false)
		}, true},
		{"empty agent_id, indented payload", false, func(t *testing.T, repo string) string {
			return agentPayload(t, filepath.Join(repo, "a b.go"), "", true)
		}, true},
	}

	for source, read := range hookCheckSources {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			taskfile := read(t)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					repo := newHookCheckRepo(t, taskfile, !tc.noGit)
					if got, want := sortedCopy(repo.stubs), hookCheckWant[source]; !slices.Equal(got, want) {
						t.Errorf("hook:check runs %q, want %q (typecheck and lint, no tests)", got, want)
					}
					payload := tc.payload(t, repo.dir)
					code, stdout, stderr := repo.run(t, task, payload)
					if code != 0 {
						t.Fatalf("exit %d, want 0; stderr: %s\npayload: %s", code, stderr, payload)
					}
					if stdout != "" {
						t.Errorf("stdout = %q; agents parse stdout on exit 0", stdout)
					}
					if strings.Contains(stderr, "tested: ") {
						t.Errorf("hook:check ran a test task:\n%s", stderr)
					}
					for _, name := range repo.stubs {
						ran := strings.Contains(stderr, "checked: "+name+"\n") || strings.Contains(stderr, "checked: "+name+"\r\n")
						if ran != tc.checks {
							t.Errorf("%s ran = %v, want %v\npayload: %s\nstderr: %s", name, ran, tc.checks, payload, stderr)
						}
					}
				})
			}
		})
	}
}
