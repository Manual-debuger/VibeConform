package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/state"
)

const (
	tsVSCode = "standard: prod-ts\nversion: v1\nintegrations:\n  editors: [vscode]\n"
	tsNoEd   = "standard: prod-ts\nversion: v1\n"
)

const createdTasks = `{
  "version": "2.0.0",
  "tasks": [
    {"label": "task fmt", "type": "shell", "command": "task", "args": ["fmt"], "problemMatcher": []},
    {"label": "task lint", "type": "shell", "command": "task", "args": ["lint"], "problemMatcher": []},
    {"label": "task test", "type": "shell", "command": "task", "args": ["test"], "problemMatcher": []},
    {"label": "task verify", "type": "shell", "command": "task", "args": ["verify"], "problemMatcher": []}
  ]
}
`

// userTasks is a tasks.json a team already has: a comment, its own task,
// four-space indentation, and a trailing comma.
const userTasks = `// Team tasks.
{
    "version": "2.0.0",
    "tasks": [
        {
            "label": "release", // ours
            "type": "shell",
            "command": "./release.sh",
        },
    ],
}
`

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeTasksJSON(t *testing.T, dir, body string) {
	t.Helper()
	p := filepath.Join(dir, ".vscode", "tasks.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustSync(t *testing.T, dir string) string {
	t.Helper()
	out, err := runSyncIn(t, dir)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	return out
}

func mustConform(t *testing.T, dir string) {
	t.Helper()
	if out, err := runAuditIn(t, dir); err != nil {
		t.Fatalf("audit: %v\n%s", err, out)
	}
}

func TestVSCodeCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, tsVSCode)
	out := mustSync(t, dir)
	for _, want := range []string{
		".vscode/tasks.json: created",
		`.vscode/tasks.json: added "task verify"`,
		`.vscode/extensions.json: added "esbenp.prettier-vscode"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sync output missing %q\n%s", want, out)
		}
	}
	if got := readFile(t, dir, ".vscode/tasks.json"); got != createdTasks {
		t.Errorf("tasks.json\n%s\nwant\n%s", got, createdTasks)
	}
	want := "{\n  \"recommendations\": [\n    \"dbaeumer.vscode-eslint\",\n    \"esbenp.prettier-vscode\"\n  ]\n}\n"
	if got := readFile(t, dir, ".vscode/extensions.json"); got != want {
		t.Errorf("extensions.json\n%s\nwant\n%s", got, want)
	}

	s, _ := state.Load(dir)
	rs := s.Resources[".vscode/tasks.json"]
	if !rs.Created || len(rs.Elements) != 4 || rs.SHA256 != "" {
		t.Errorf("state entry %+v", rs)
	}
	if _, ok := rs.Elements["tasks/task verify"]; !ok {
		t.Errorf("no tasks/task verify element in %v", rs.Elements)
	}

	mustConform(t, dir)
	if out := mustSync(t, dir); strings.Contains(out, "added \"") || strings.Contains(out, "updated \"") {
		t.Errorf("second sync changed something\n%s", out)
	}
}

// TestVSCodeMergesIntoUserFile: every byte of the team's file survives,
// and their later additions are not drift.
func TestVSCodeMergesIntoUserFile(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, tsVSCode)
	writeTasksJSON(t, dir, userTasks)
	mustSync(t, dir)

	got := readFile(t, dir, ".vscode/tasks.json")
	head := `// Team tasks.
{
    "version": "2.0.0",
    "tasks": [
        {
            "label": "release", // ours
            "type": "shell",
            "command": "./release.sh",
        },
        {"label": "task fmt",`
	if !strings.HasPrefix(got, head) || !strings.HasSuffix(got, "\"problemMatcher\": []},\n    ],\n}\n") {
		t.Errorf("user content not preserved:\n%s", got)
	}
	mustConform(t, dir)

	// The team adds a task of their own: still conformant.
	edited := strings.Replace(got, "    ],\n}", "        {\"label\": \"deploy\"},\n    ],\n}", 1)
	writeTasksJSON(t, dir, edited)
	mustConform(t, dir)

	// Someone edits one of ours: drift, which sync restores in place.
	drifted := strings.Replace(edited, `"args": ["lint"]`, `"args": ["lint", "--fix"]`, 1)
	writeTasksJSON(t, dir, drifted)
	if out, err := runAuditIn(t, dir); ExitCode(err) != exitNonConformant || !strings.Contains(out, `drifted "task lint"`) {
		t.Errorf("audit exit %d\n%s", ExitCode(err), out)
	}
	if out := mustSync(t, dir); !strings.Contains(out, `updated "task lint"`) {
		t.Errorf("sync\n%s", out)
	}
	if readFile(t, dir, ".vscode/tasks.json") != edited {
		t.Error("restoring the element changed anything else")
	}

	// Deselecting removes exactly our elements: the team's file is back to
	// what they wrote, plus their deploy task.
	writeVibeYAML(t, dir, tsNoEd)
	if diff := runDiffIn(t, dir); !strings.Contains(diff, `.vscode/tasks.json: would remove "task verify" (vscode deselected)`) {
		t.Errorf("diff\n%s", diff)
	}
	mustSync(t, dir)
	want := strings.Replace(userTasks, "    ],\n}", "        {\"label\": \"deploy\"},\n    ],\n}", 1)
	if got := readFile(t, dir, ".vscode/tasks.json"); got != want {
		t.Errorf("after deselect\n%s\nwant\n%s", got, want)
	}
	s, _ := state.Load(dir)
	if _, ok := s.Resources[".vscode/tasks.json"]; ok {
		t.Error("state still records tasks.json")
	}
	mustConform(t, dir)
}

func TestVSCodeDeselectDeletesCreatedFiles(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, tsVSCode)
	mustSync(t, dir)
	writeVibeYAML(t, dir, tsNoEd)
	out := mustSync(t, dir)
	if !strings.Contains(out, ".vscode/tasks.json: removed (vscode deselected; nothing else was in it)") {
		t.Errorf("sync\n%s", out)
	}
	if exists(t, dir, ".vscode") {
		t.Error(".vscode/ left behind")
	}
	mustConform(t, dir)
}

func TestVSCodeLabelCollisionIsAConflict(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, tsVSCode)
	mine := "{\"version\": \"2.0.0\", \"tasks\": [{\"label\": \"task verify\", \"command\": \"make\"}]}\n"
	writeTasksJSON(t, dir, mine)
	if out, err := runAuditIn(t, dir); ExitCode(err) != exitNonConformant || !strings.Contains(out, `conflict: "task verify"`) {
		t.Errorf("audit exit %d\n%s", ExitCode(err), out)
	}
	if _, err := runSyncIn(t, dir); err == nil {
		t.Error("sync succeeded over a label collision")
	}
	if readFile(t, dir, ".vscode/tasks.json") != mine {
		t.Error("a conflicted file was written")
	}
}

func TestVSCodeDeselectKeepsModifiedElement(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, tsVSCode)
	mustSync(t, dir)
	edited := strings.Replace(readFile(t, dir, ".vscode/tasks.json"), `"args": ["test"]`, `"args": ["test", "-v"]`, 1)
	writeTasksJSON(t, dir, edited)
	writeVibeYAML(t, dir, tsNoEd)

	if _, err := runSyncIn(t, dir); err == nil {
		t.Error("sync succeeded over a modified deselected element")
	}
	if readFile(t, dir, ".vscode/tasks.json") != edited {
		t.Error("the file with a modified element was changed")
	}
	if out, err := runAuditIn(t, dir); ExitCode(err) != exitNonConformant {
		t.Errorf("audit exit %d\n%s", ExitCode(err), out)
	}
}

func TestVSCodeInvalidFileIsAConflict(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, tsVSCode)
	writeTasksJSON(t, dir, "{ not json")
	out, err := runAuditIn(t, dir)
	if ExitCode(err) != exitNonConformant || !strings.Contains(out, ".vscode/tasks.json: conflict: not valid JSON with comments") {
		t.Errorf("audit exit %d\n%s", ExitCode(err), out)
	}
}

// TestRecommendationsFollowProfiles: a monorepo's recommendations cover
// every declared profile, and dropping a component's profile prunes its
// recommendations as out of date.
func TestRecommendationsFollowProfiles(t *testing.T) {
	dir := t.TempDir()
	both := "standard: prod-mono\nversion: v1\ncomponents:\n  - {id: api, path: api, profile: go}\n  - {id: web, path: web, profile: ts}\n" +
		"integrations:\n  editors: [zed, vscode]\n  agents: [claude]\n"
	writeVibeYAML(t, dir, both)
	mustSync(t, dir)
	ext := readFile(t, dir, ".vscode/extensions.json")
	for _, id := range []string{"golang.go", "dbaeumer.vscode-eslint", "esbenp.prettier-vscode"} {
		if !strings.Contains(ext, id) {
			t.Errorf("extensions.json lacks %s:\n%s", id, ext)
		}
	}
	if !strings.Contains(readFile(t, dir, ".zed/tasks.json"), `"label": "task verify"`) {
		t.Error("zed tasks missing")
	}

	goOnly := "standard: prod-mono\nversion: v1\ncomponents:\n  - {id: api, path: api, profile: go}\n" +
		"integrations:\n  editors: [zed, vscode]\n  agents: [claude]\n"
	writeVibeYAML(t, dir, goOnly)
	if out, err := runAuditIn(t, dir); !strings.Contains(out, `.vscode/extensions.json: out of date ("esbenp.prettier-vscode" no longer owned`) {
		t.Errorf("audit (err %v)\n%s", err, out)
	}
	mustSync(t, dir)
	if got, want := readFile(t, dir, ".vscode/extensions.json"), "{\n  \"recommendations\": [\n    \"golang.go\"\n  ]\n}\n"; got != want {
		t.Errorf("extensions.json\n%s\nwant\n%s", got, want)
	}
}

func TestZedTopLevelArray(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, "standard: prod-go\nversion: v1\nintegrations:\n  editors: [zed]\n")
	mustSync(t, dir)
	want := "[\n" +
		"  {\"label\": \"task fmt\", \"command\": \"task\", \"args\": [\"fmt\"]},\n" +
		"  {\"label\": \"task lint\", \"command\": \"task\", \"args\": [\"lint\"]},\n" +
		"  {\"label\": \"task test\", \"command\": \"task\", \"args\": [\"test\"]},\n" +
		"  {\"label\": \"task verify\", \"command\": \"task\", \"args\": [\"verify\"]}\n" +
		"]\n"
	if got := readFile(t, dir, ".zed/tasks.json"); got != want {
		t.Errorf("zed tasks.json\n%s\nwant\n%s", got, want)
	}
	s, _ := state.Load(dir)
	if _, ok := s.Resources[".zed/tasks.json"].Elements["[]/task fmt"]; !ok {
		t.Errorf("state %+v", s.Resources[".zed/tasks.json"])
	}
	mustConform(t, dir)
}
