package monorepotooling

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/resource"
)

var allProfiles = []manifest.Component{
	{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
	{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
	{ID: "worker", Path: "services/worker", Profile: manifest.ProfilePy},
}

// singleLanguage is each profile's single-language repo-tooling Taskfile,
// whose commands a component's Taskfile must repeat verbatim.
var singleLanguage = map[manifest.Profile]string{
	manifest.ProfileGo: "../repotooling/templates/Taskfile.yml",
	manifest.ProfileTS: "../tsrepotooling/templates/Taskfile.yml",
	manifest.ProfilePy: "../pyrepotooling/templates/Taskfile.yml",
}

// taskfile is the part of a Taskfile these tests read. Cmds are kept as
// raw values: a string is a shell command, a map a {task: ...} call.
type taskfile struct {
	Includes map[string]struct {
		Taskfile string `yaml:"taskfile"`
		Dir      string `yaml:"dir"`
		Optional bool   `yaml:"optional"`
	} `yaml:"includes"`
	Tasks map[string]struct {
		Desc string `yaml:"desc"`
		Cmds []any  `yaml:"cmds"`
	} `yaml:"tasks"`
}

func parse(t *testing.T, name string, data []byte) taskfile {
	t.Helper()
	var tf taskfile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		t.Fatalf("parsing %s: %v\n%s", name, err, data)
	}
	return tf
}

func resolve(t *testing.T, components []manifest.Component) map[string]resource.Resource {
	t.Helper()
	rs, err := New().Resolve(context.Background(), &module.Context{Components: components})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	out := map[string]resource.Resource{}
	for _, r := range rs {
		out[r.Path] = r
	}
	return out
}

// TestComponentTaskfilesMatchSingleLanguage is the drift alarm between a
// component's Taskfile and its single-language standard's (spec 0025): a
// task both define runs the same commands, and fmt:changed runs exactly
// what the single-language hook:format does.
func TestComponentTaskfilesMatchSingleLanguage(t *testing.T) {
	for p, prof := range profiles {
		t.Run(string(p), func(t *testing.T) {
			data, err := os.ReadFile(filepath.FromSlash(singleLanguage[p]))
			if err != nil {
				t.Fatal(err)
			}
			single := parse(t, singleLanguage[p], data)
			component := parse(t, string(p)+".Taskfile.yml", prof.taskfile)

			for name, task := range component.Tasks {
				want, ok := single.Tasks[name]
				if name == "fmt:changed" {
					want, ok = single.Tasks["hook:format"]
				}
				if !ok {
					t.Errorf("component task %s has no single-language counterpart", name)
					continue
				}
				if name == "verify" || name == "verify:fast" {
					continue // compared by TestComponentVerifyMatchesSingleLanguage
				}
				if !reflect.DeepEqual(task.Cmds, want.Cmds) {
					t.Errorf("%s: cmds\n%v\nwant (single-language)\n%v", name, task.Cmds, want.Cmds)
				}
			}
		})
	}
}

// TestComponentVerifyMatchesSingleLanguage: a component's verify and
// verify:fast run the single-language steps, less workflows:lint, which
// only makes sense once per repository and runs from the root.
func TestComponentVerifyMatchesSingleLanguage(t *testing.T) {
	for p, prof := range profiles {
		t.Run(string(p), func(t *testing.T) {
			data, err := os.ReadFile(filepath.FromSlash(singleLanguage[p]))
			if err != nil {
				t.Fatal(err)
			}
			single := parse(t, singleLanguage[p], data)
			component := parse(t, string(p)+".Taskfile.yml", prof.taskfile)
			for _, name := range []string{"verify", "verify:fast"} {
				want := slices.DeleteFunc(slices.Clone(single.Tasks[name].Cmds), func(c any) bool {
					m, ok := c.(map[string]any)
					return ok && m["task"] == "workflows:lint"
				})
				if got := component.Tasks[name].Cmds; !reflect.DeepEqual(got, want) {
					t.Errorf("%s: cmds %v, want %v", name, got, want)
				}
			}
		})
	}
}

// TestGuardCommandMatchesSingleLanguage: hook:guard runs the guard exactly
// as the single-language standard that owns it does.
func TestGuardCommandMatchesSingleLanguage(t *testing.T) {
	for p, prof := range profiles {
		data, err := os.ReadFile(filepath.FromSlash(singleLanguage[p]))
		if err != nil {
			t.Fatal(err)
		}
		single := parse(t, singleLanguage[p], data)
		want := prof.guardCommand + " || exit 2"
		if got := single.Tasks["hook:guard"].Cmds; len(got) != 1 || got[0] != want {
			t.Errorf("%s: single-language hook:guard is %v, want [%s]", p, got, want)
		}
	}
}

// TestGuardRuntimeOrder: the guard runs in the first runtime the
// repository has, in the order Go, Node, Python.
func TestGuardRuntimeOrder(t *testing.T) {
	cases := []struct {
		components []manifest.Component
		want       string
	}{
		{allProfiles, ".claude/hooks/guard.go"},
		{allProfiles[1:], ".claude/hooks/guard.mjs"},
		{allProfiles[2:], ".claude/hooks/guard.py"},
		{[]manifest.Component{allProfiles[2], allProfiles[0]}, ".claude/hooks/guard.go"},
	}
	for _, tc := range cases {
		rs := resolve(t, tc.components)
		var guards []string
		for path := range rs {
			if strings.HasPrefix(path, ".claude/hooks/guard.") {
				guards = append(guards, path)
			}
		}
		if len(guards) != 1 || guards[0] != tc.want {
			t.Errorf("%v: guards %v, want [%s]", tc.components, guards, tc.want)
			continue
		}
		root := parse(t, "Taskfile.yml", rs["Taskfile.yml"].Content)
		cmd, _ := root.Tasks["hook:guard"].Cmds[0].(string)
		if !strings.Contains(cmd, tc.want) {
			t.Errorf("hook:guard runs %q, not %s", cmd, tc.want)
		}
	}
}

// TestResolveOrder pins resource order, which audit, diff, and sync report
// in: root files, then each component's Taskfile in vibe.yaml order.
func TestResolveOrder(t *testing.T) {
	rs, err := New().Resolve(context.Background(), &module.Context{Components: allProfiles})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rs {
		got = append(got, r.Path)
		if r.Ownership != resource.Generated {
			t.Errorf("%s: ownership %v, want generated", r.Path, r.Ownership)
		}
	}
	want := []string{
		"Taskfile.yml", "lefthook.yml", ".claude/hooks/guard.go",
		"services/api/Taskfile.yml", "apps/web/Taskfile.yml", "services/worker/Taskfile.yml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resources %v, want %v", got, want)
	}
}

func TestResolveNeedsComponents(t *testing.T) {
	for _, mctx := range []*module.Context{nil, {}} {
		if _, err := New().Resolve(context.Background(), mctx); err == nil {
			t.Errorf("Resolve(%v) succeeded with no components", mctx)
		}
	}
}

// TestRootTaskfile checks the rendered root: each component is included
// under its id from its own directory, and every root task fans out to
// every component in vibe.yaml order.
func TestRootTaskfile(t *testing.T) {
	rs := resolve(t, allProfiles)
	root := parse(t, "Taskfile.yml", rs["Taskfile.yml"].Content)

	for _, c := range allProfiles {
		inc, ok := root.Includes[c.ID]
		if !ok {
			t.Errorf("no include for %s", c.ID)
			continue
		}
		if inc.Taskfile != "./"+c.Path+"/Taskfile.yml" || inc.Dir != "./"+c.Path || inc.Optional {
			t.Errorf("include %s = %+v", c.ID, inc)
		}
		if _, ok := rs[c.Path+"/Taskfile.yml"]; !ok {
			t.Errorf("include %s names a Taskfile the module does not write", c.ID)
		}
	}

	for _, name := range []string{"fmt", "fmt:check", "lint", "typecheck", "test", "verify", "verify:fast"} {
		task, ok := root.Tasks[name]
		if !ok {
			t.Errorf("root defines no %s", name)
			continue
		}
		if task.Desc == "" {
			t.Errorf("%s has no desc, so task --list hides it", name)
		}
		var want []any
		for _, c := range allProfiles {
			want = append(want, map[string]any{"task": c.ID + ":" + name})
		}
		if name == "verify" {
			want = append(want, map[string]any{"task": "workflows:lint"})
		}
		if !reflect.DeepEqual(task.Cmds, want) {
			t.Errorf("%s: cmds %v, want %v", name, task.Cmds, want)
		}
	}

	for _, name := range []string{"hook:guard", "hook:context", "hook:format", "hook:check", "hook:done", "verify-ci", "workflows:lint"} {
		if _, ok := root.Tasks[name]; !ok {
			t.Errorf("root defines no %s", name)
		}
	}

	format, _ := root.Tasks["hook:format"].Cmds[0].(string)
	check, _ := root.Tasks["hook:check"].Cmds[0].(string)
	for _, c := range allProfiles {
		if !strings.Contains(format, "(cd "+c.Path+" && task -s fmt:changed) || rc=2") {
			t.Errorf("hook:format does not format %s from its own directory:\n%s", c.ID, format)
		}
		for _, step := range []string{"typecheck", "lint", "test"} {
			if !strings.Contains(check, " "+c.ID+":"+step) {
				t.Errorf("hook:check does not run %s:%s", c.ID, step)
			}
		}
	}
}

// TestNothingRunsVibe extends spec 0017/0022 to prod-mono: no generated
// Taskfile or hook calls vibe; only Taskfile.vibe.yml does.
func TestNothingRunsVibe(t *testing.T) {
	for path, r := range resolve(t, allProfiles) {
		if strings.HasPrefix(path, ".claude/") {
			continue
		}
		for line := range strings.Lines(string(r.Content)) {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			if strings.Contains(line, "vibe ") || strings.Contains(line, "vibe audit") {
				t.Errorf("%s runs vibe: %s", path, line)
			}
		}
	}
}

// TestHookContextListsOnlyDeclaredRuntimes: session context names the
// toolchains the repository uses and no others.
func TestHookContextListsOnlyDeclaredRuntimes(t *testing.T) {
	lines := map[manifest.Profile][]string{
		manifest.ProfileGo: {`echo "Go: `},
		manifest.ProfileTS: {`echo "Node: `, `echo "pnpm: `},
		manifest.ProfilePy: {`echo "uv: `, `echo "Python: `},
	}
	for _, c := range allProfiles {
		root := parse(t, "Taskfile.yml", resolve(t, []manifest.Component{c})["Taskfile.yml"].Content)
		script, _ := root.Tasks["hook:context"].Cmds[0].(string)
		for p, want := range lines {
			for _, w := range want {
				if got := strings.Contains(script, w); got != (p == c.Profile) {
					t.Errorf("%s component: hook:context contains %q = %v", c.Profile, w, got)
				}
			}
		}
		if !strings.Contains(script, "  "+c.ID+": "+c.Path+" ("+string(c.Profile)+")") {
			t.Errorf("hook:context does not list component %s", c.ID)
		}
	}
}

// TestLefthook: one pre-commit check pair per component, run from its
// directory on its own language's files.
func TestLefthook(t *testing.T) {
	var doc struct {
		PreCommit struct {
			Commands map[string]struct {
				Root string `yaml:"root"`
				Glob string `yaml:"glob"`
				Run  string `yaml:"run"`
			} `yaml:"commands"`
		} `yaml:"pre-commit"`
		PrePush struct {
			Commands map[string]struct {
				Run string `yaml:"run"`
			} `yaml:"commands"`
		} `yaml:"pre-push"`
	}
	if err := yaml.Unmarshal(resolve(t, allProfiles)["lefthook.yml"].Content, &doc); err != nil {
		t.Fatal(err)
	}
	if n := len(doc.PreCommit.Commands); n != 2*len(allProfiles) {
		t.Errorf("%d pre-commit commands, want %d", n, 2*len(allProfiles))
	}
	for _, c := range allProfiles {
		for suffix, run := range map[string]string{"-fmt": "task fmt:check", "-lint": "task lint"} {
			cmd, ok := doc.PreCommit.Commands[c.ID+suffix]
			if !ok {
				t.Errorf("no pre-commit %s%s", c.ID, suffix)
				continue
			}
			if cmd.Root != c.Path+"/" || cmd.Glob != profiles[c.Profile].glob || cmd.Run != run {
				t.Errorf("pre-commit %s%s = %+v", c.ID, suffix, cmd)
			}
		}
	}
	if doc.PrePush.Commands["verify-fast"].Run != "task verify:fast" {
		t.Errorf("pre-push = %+v, want task verify:fast", doc.PrePush.Commands)
	}
}

// TestRequiredToolsFollowProfiles: a repository is warned only about the
// toolchains it declares.
func TestRequiredToolsFollowProfiles(t *testing.T) {
	names := func(components []manifest.Component) []string {
		var out []string
		for _, tool := range New().(module.ToolRequirer).RequiredTools(&module.Context{Components: components}) {
			out = append(out, tool.Name)
		}
		return out
	}
	if got, want := names(allProfiles[2:]), []string{"task", "lefthook", "uv", "actionlint"}; !reflect.DeepEqual(got, want) {
		t.Errorf("py only: %v, want %v", got, want)
	}
	if got, want := names(allProfiles), []string{"task", "lefthook", "go", "goimports", "govulncheck", "pnpm", "uv", "actionlint"}; !reflect.DeepEqual(got, want) {
		t.Errorf("all: %v, want %v", got, want)
	}
}

// TestDeterministic: the same components render the same bytes.
func TestDeterministic(t *testing.T) {
	a, b := resolve(t, allProfiles), resolve(t, allProfiles)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two resolutions of the same components differ")
	}
}
