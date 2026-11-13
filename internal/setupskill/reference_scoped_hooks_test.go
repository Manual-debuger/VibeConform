package setupskill_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The monorepo edit hooks scoped to the edited component (spec 0060,
// Desired Behavior 1-6, Seam 1; #81): after an edit inside a component,
// the PostToolUse hooks format, type-check and lint that component only;
// any other edit, and task fmt without a hook payload, runs every
// component.

// monorepoTaskfile is the reference monorepo root Taskfile.
const monorepoTaskfile = "reference/taskrunner/monorepo/Taskfile.yml"

// componentTable parses hook:owner's COMPONENTS ("id=path ...") into id
// and path pairs, in table order.
func componentTable(t *testing.T, taskfile []byte) [][2]string {
	t.Helper()
	var doc struct {
		Tasks map[string]struct {
			Vars map[string]string `yaml:"vars"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(taskfile, &doc); err != nil {
		t.Fatal(err)
	}
	table := doc.Tasks["hook:owner"].Vars["COMPONENTS"]
	if table == "" {
		t.Fatal("hook:owner has no COMPONENTS table")
	}
	var out [][2]string
	for entry := range strings.FieldsSeq(table) {
		id, path, ok := strings.Cut(entry, "=")
		if !ok || id == "" || path == "" {
			t.Fatalf("COMPONENTS entry %q is not id=path", entry)
		}
		out = append(out, [2]string{id, path})
	}
	return out
}

// TestComponentTableMatchesTheManifest pins Desired Behavior 6: the
// component table names the components of the reference vibe.yaml, and
// each id is the include the root Taskfile runs from that path.
func TestComponentTableMatchesTheManifest(t *testing.T) {
	t.Parallel()
	taskfile := readRef(t, monorepoTaskfile)
	var root struct {
		Includes map[string]struct {
			Dir string `yaml:"dir"`
		} `yaml:"includes"`
	}
	if err := yaml.Unmarshal(taskfile, &root); err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Components []struct {
			Path string `yaml:"path"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(readRef(t, "reference/vibe-yaml/monorepo.yaml"), &manifest); err != nil {
		t.Fatal(err)
	}
	var tablePaths, manifestPaths []string
	for _, entry := range componentTable(t, taskfile) {
		id, path := entry[0], entry[1]
		tablePaths = append(tablePaths, path)
		inc, ok := root.Includes[id]
		if !ok {
			t.Errorf("COMPONENTS names %s, which the root Taskfile does not include", id)
		} else if got := strings.TrimPrefix(inc.Dir, "./"); got != path {
			t.Errorf("COMPONENTS has %s=%s, but the include %s runs in %s", id, path, id, inc.Dir)
		}
	}
	for _, c := range manifest.Components {
		manifestPaths = append(manifestPaths, c.Path)
	}
	slices.Sort(tablePaths)
	slices.Sort(manifestPaths)
	if !slices.Equal(tablePaths, manifestPaths) {
		t.Errorf("COMPONENTS paths %v, vibe.yaml components %v", tablePaths, manifestPaths)
	}
}

// TestMonorepoEditHooksRunTheEditedComponent runs the reference
// PostToolUse commands, as Claude Code does, for edits in and out of the
// components (Desired Behavior 1, 3 and 4).
func TestMonorepoEditHooksRunTheEditedComponent(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the reference hooks for each edit; task test runs it")
	}
	bash := hookShell(t)
	r := newHookRepo(t)
	r.write(t, "apps/web2/x.ts", "export {};\n")
	r.write(t, "services/worker/app/nb.ipynb", "{}\n")
	format := r.command(t, "PostToolUse", "task fmt")
	check := r.command(t, "PostToolUse", "hook:check")

	edit := func(key string, rel ...string) func() string {
		return func() string {
			return marshalPayload(t, map[string]any{"session_id": "s", "hook_event_name": "PostToolUse", "tool_name": "Edit",
				"tool_input": map[string]any{key: filepath.Join(append([]string{r.root}, rel...)...)}}, false)
		}
	}
	every := func(task ...string) []string {
		var out []string
		for _, name := range task {
			out = append(out, each(name)...)
		}
		slices.Sort(out)
		return out
	}
	only := func(id string) ([]string, []string) {
		return []string{id + ":fmt"}, []string{id + ":lint", id + ":typecheck"}
	}

	type editCase struct {
		name           string
		payload        func() string
		fmtRan, chkRan []string
	}
	apiFmt, apiChk := only("api")
	webFmt, webChk := only("web")
	workerFmt, workerChk := only("worker")
	cases := []editCase{
		{"in api", edit("file_path", "services", "api", "main.go"), apiFmt, apiChk},
		{"in a subdirectory of api", edit("file_path", "services", "api", "internal", "calc", "calc.go"), apiFmt, apiChk},
		{"in web", edit("file_path", "apps", "web", "src", "add.ts"), webFmt, webChk},
		{"in worker", edit("file_path", "services", "worker", "app", "__init__.py"), workerFmt, workerChk},
		{"a notebook in worker", edit("notebook_path", "services", "worker", "app", "nb.ipynb"), workerFmt, workerChk},
		{"in web, indented payload", func() string {
			return marshalPayload(t, map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Write",
				"tool_input": map[string]any{"file_path": filepath.Join(r.root, "apps", "web", "src", "add.ts")}}, true)
		}, webFmt, webChk},
		{"a root file", edit("file_path", "Taskfile.yml"), every("fmt"), every("lint", "typecheck")},
		{"in no component", edit("file_path", "docs", "README.md"), every("fmt"), every("lint", "typecheck")},
		{"a sibling with a shared prefix", edit("file_path", "apps", "web2", "x.ts"), every("fmt"), every("lint", "typecheck")},
		{"a directory that does not exist", edit("file_path", "services", "api", "gone", "x.go"), every("fmt"), every("lint", "typecheck")},
		{"no path", func() string {
			return `{"hook_event_name":"PostToolUse","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch"}}`
		}, every("fmt"), every("lint", "typecheck")},
		{"empty stdin", func() string { return "" }, every("fmt"), every("lint", "typecheck")},
		{"not JSON", func() string { return "not json" }, every("fmt"), every("lint", "typecheck")},
		// A sub-agent's edit is formatted in its component; hook:check
		// skips it (spec 0057).
		{"a sub-agent's edit in api", func() string {
			return agentPayload(t, filepath.Join(r.root, "services", "api", "main.go"), "a1b2c3", false)
		}, apiFmt, nil},
	}
	if runtime.GOOS == "windows" {
		// Windows paths ignore case, and the shell resolves the path's
		// directory to the case on disk.
		cases = append(cases, editCase{"in web, lower-case path", func() string {
			return editPayload(t, "file_path", strings.ToLower(filepath.Join(r.root, "apps", "web", "src", "add.ts")), false)
		}, webFmt, webChk})
	}
	for _, tc := range cases {
		payload := tc.payload()
		for _, hook := range []struct {
			name, command string
			want          []string
		}{{"format", format, tc.fmtRan}, {"check", check, tc.chkRan}} {
			got, stderr := r.runInput(t, bash, ".", hook.command, payload, r.env)
			if got.code != 0 {
				t.Errorf("%s, %s: exit %d, want 0\nstderr: %s", tc.name, hook.name, got.code, stderr)
			}
			if !slices.Equal(got.ran, hook.want) {
				t.Errorf("%s, %s: ran %v, want %v\npayload: %s\nstderr: %s", tc.name, hook.name, got.ran, hook.want, payload, stderr)
			}
		}
	}
}

// TestFmtWithoutAHookRunsEveryComponent pins Desired Behavior 3 and the
// fmt entrypoint's meaning (Constraints): a person, CI or a Git hook that
// runs task fmt formats every component, even when stdin holds an edit
// payload, and nothing waits for input.
func TestFmtWithoutAHookRunsEveryComponent(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the reference fmt; task test runs it")
	}
	bash := hookShell(t)
	r := newHookRepo(t)
	var noHook []string
	for _, kv := range r.env {
		if !strings.HasPrefix(kv, "CLAUDE_PROJECT_DIR=") {
			noHook = append(noHook, kv)
		}
	}
	payload, err := json.Marshal(map[string]any{"tool_input": map[string]any{"file_path": filepath.Join(r.root, "services", "api", "main.go")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, stdin string }{
		{"no stdin", ""},
		{"an edit payload outside a hook", string(payload)},
	} {
		got, stderr := r.runInput(t, bash, ".", "task fmt", tc.stdin, noHook)
		if got.code != 0 || !slices.Equal(got.ran, each("fmt")) {
			t.Errorf("%s: exit %d, ran %v; want exit 0 and %v\nstderr: %s", tc.name, got.code, got.ran, each("fmt"), stderr)
		}
	}
}

// TestHookOwnerPicksTheLongestWholeSegmentPath pins Desired Behavior 2
// and the segment rule with a component table that nests: the longest
// path owns a file, and apps/web2 is not inside apps/web.
func TestHookOwnerPicksTheLongestWholeSegmentPath(t *testing.T) {
	t.Parallel()
	task := taskBinary(t)
	r := newHookRepo(t)
	taskfile := string(readRef(t, monorepoTaskfile))
	const table = "COMPONENTS: api=services/api web=apps/web worker=services/worker"
	if !strings.Contains(taskfile, table) {
		t.Fatalf("the reference Taskfile has no line %q", table)
	}
	r.write(t, "Taskfile.yml", strings.Replace(taskfile, table,
		"COMPONENTS: api=services/api calc=services/api/internal/calc web=apps/web", 1))
	r.write(t, "apps/web2/x.ts", "export {};\n")
	for _, tc := range []struct {
		rel  []string
		want string
	}{
		{[]string{"services", "api", "main.go"}, "api"},
		{[]string{"services", "api", "internal", "calc", "calc.go"}, "calc"},
		{[]string{"apps", "web", "src", "add.ts"}, "web"},
		{[]string{"apps", "web2", "x.ts"}, ""},
		{[]string{"services", "worker", "app", "__init__.py"}, ""},
	} {
		payload := editPayload(t, "file_path", filepath.Join(append([]string{r.root}, tc.rel...)...), false)
		// #nosec G204 -- task is the binary on PATH, the arguments are fixed
		cmd := exec.Command(task, "-s", "hook:owner")
		cmd.Dir, cmd.Env = r.root, r.env
		cmd.Stdin = strings.NewReader(payload)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("hook:owner for %s: %v", filepath.Join(tc.rel...), err)
		}
		if got := strings.TrimSpace(string(out)); got != tc.want {
			t.Errorf("hook:owner for %s = %q, want %q", filepath.Join(tc.rel...), got, tc.want)
		}
	}
}
