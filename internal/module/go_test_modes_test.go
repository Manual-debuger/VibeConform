package module_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/manifest"
	"github.com/Manual-debuger/VibeConform/internal/module"
	"github.com/Manual-debuger/VibeConform/internal/module/monorepotooling"
)

// Spec 0056 Desired Behavior 2 and 3, for every Taskfile that runs go test:
// the fast checks (verify:fast, hook:check) run the tests in short mode, and
// the full runs (test, test:race, verify, and the pre-push hook) run every
// test with an explicit 30-minute timeout, so nobody needs GOFLAGS to push.

// goTimeout is the timeout every full go test run passes. Go's default is
// ten minutes, which this repository's suite outgrew (#125).
const goTimeout = "30m"

// goTaskSetup is one repository layout whose Taskfiles run go test: a root
// Taskfile, the Taskfiles it includes by id, and its lefthook.yml when it
// has one. goComponents are the include ids that are Go components; a
// single-language root is the one Go "component", with id "".
type goTaskSetup struct {
	root         []byte
	includes     map[string][]byte
	lefthook     []byte
	goComponents []string
}

// monoComponents are the rendered prod-mono setup's components: two Go
// components, so the pre-push check must find each, and one of each other
// profile, whose tests are not Go's.
var monoComponents = []manifest.Component{
	{ID: "api", Path: "services/api", Profile: manifest.ProfileGo},
	{ID: "web", Path: "apps/web", Profile: manifest.ProfileTS},
	{ID: "jobs", Path: "services/jobs", Profile: manifest.ProfileGo},
	{ID: "worker", Path: "services/worker", Profile: manifest.ProfilePy},
}

// goTaskSetups are every Go setup VibeConform generates or the setup skill
// hands out.
var goTaskSetups = map[string]func(t *testing.T) goTaskSetup{
	"prod-go": func(t *testing.T) goTaskSetup {
		t.Helper()
		return goTaskSetup{
			root:         readSource("repotooling/templates/Taskfile.yml")(t),
			lefthook:     readSource("repotooling/templates/lefthook.yml")(t),
			goComponents: []string{""},
		}
	},
	"prod-mono": func(t *testing.T) goTaskSetup {
		t.Helper()
		rs, err := monorepotooling.New().Resolve(context.Background(), &module.Context{Components: monoComponents})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		files := map[string][]byte{}
		for _, r := range rs {
			files[r.Path] = r.Content
		}
		s := goTaskSetup{root: files["Taskfile.yml"], lefthook: files["lefthook.yml"], includes: map[string][]byte{}}
		for _, c := range monoComponents {
			s.includes[c.ID] = files[c.Path+"/Taskfile.yml"]
			if c.Profile == manifest.ProfileGo {
				s.goComponents = append(s.goComponents, c.ID)
			}
		}
		return s
	},
	"setupskill/reference/go": func(t *testing.T) goTaskSetup {
		t.Helper()
		return goTaskSetup{
			root:         readSource("../setupskill/skill/reference/taskrunner/go/Taskfile.yml")(t),
			lefthook:     readSource("../setupskill/skill/reference/hooks/go/lefthook.yml")(t),
			goComponents: []string{""},
		}
	},
	"setupskill/reference/monorepo": func(t *testing.T) goTaskSetup {
		t.Helper()
		dir := "../setupskill/skill/reference/taskrunner/monorepo/"
		return goTaskSetup{
			root: readSource(dir + "Taskfile.yml")(t),
			includes: map[string][]byte{
				"api":    readSource(dir + "go.Taskfile.yml")(t),
				"web":    readSource(dir + "typescript.Taskfile.yml")(t),
				"worker": readSource(dir + "python.Taskfile.yml")(t),
			},
			lefthook:     readSource("../setupskill/skill/reference/hooks/monorepo/lefthook.yml")(t),
			goComponents: []string{"api"},
		}
	},
}

// taskStep is one entry of a task's cmds: a shell script or a "task:" call.
type taskStep struct {
	shell, dep string
}

func (s *taskStep) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Decode(&s.shell)
	}
	var m struct {
		Task string `yaml:"task"`
		Cmd  string `yaml:"cmd"`
	}
	if err := n.Decode(&m); err != nil {
		return err
	}
	s.dep, s.shell = m.Task, m.Cmd
	return nil
}

// taskGraph is a Taskfile's tasks plus the Taskfiles it includes by id.
type taskGraph struct {
	tasks    map[string][]taskStep
	includes map[string]taskGraph
}

func parseTaskGraph(t *testing.T, data []byte, includes map[string][]byte) taskGraph {
	t.Helper()
	var doc struct {
		Tasks map[string]struct {
			Cmds []taskStep `yaml:"cmds"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing Taskfile: %v", err)
	}
	g := taskGraph{tasks: map[string][]taskStep{}, includes: map[string]taskGraph{}}
	for name, task := range doc.Tasks {
		g.tasks[name] = task.Cmds
	}
	for id, inc := range includes {
		g.includes[id] = parseTaskGraph(t, inc, nil)
	}
	return g
}

func (s goTaskSetup) graph(t *testing.T) taskGraph {
	t.Helper()
	return parseTaskGraph(t, s.root, s.includes)
}

// goTest is one go test command a task runs, and the include it runs in
// ("" for the root).
type goTest struct {
	component string
	command   string
}

// goTests returns every go test command the named task runs: its own, those
// of the tasks it calls with "- task:", and those of the tasks its scripts
// run with "task -s <names> 1>&2". A name with an include's id
// ("api:test") resolves in that include.
func (g taskGraph) goTests(t *testing.T, name string) []goTest {
	t.Helper()
	return g.walk(t, "", name, map[string]bool{})
}

func (g taskGraph) walk(t *testing.T, component, name string, seen map[string]bool) []goTest {
	t.Helper()
	if id, rest, ok := strings.Cut(name, ":"); ok {
		if inc, ok := g.includes[id]; ok {
			return inc.walk(t, id, rest, seen)
		}
	}
	if seen[component+"/"+name] {
		return nil
	}
	seen[component+"/"+name] = true
	steps, ok := g.tasks[name]
	if !ok {
		t.Fatalf("task %q is called but not defined", name)
	}
	var out []goTest
	for _, s := range steps {
		if s.dep != "" {
			out = append(out, g.walk(t, component, s.dep, seen)...)
			continue
		}
		for _, cmd := range module.GoTestCommands(s.shell) {
			out = append(out, goTest{component, cmd})
		}
		if m := checkTasks.FindStringSubmatch(s.shell); m != nil {
			for _, called := range strings.Fields(m[1]) {
				out = append(out, g.walk(t, component, called, seen)...)
			}
		}
	}
	return out
}

// goFlag reports whether a go test command passes flag, and its value:
// "-timeout 30m" and "-timeout=30m" both give "30m".
func goFlag(command, flag string) (string, bool) {
	fields := strings.Fields(command)
	for i, f := range fields {
		if v, ok := strings.CutPrefix(f, flag+"="); ok {
			return v, true
		}
		if f == flag {
			if flag == "-short" || i+1 == len(fields) {
				return "", true
			}
			return fields[i+1], true
		}
	}
	return "", false
}

// components returns the include ids the go tests run in.
func components(tests []goTest) []string {
	var ids []string
	for _, gt := range tests {
		if !slices.Contains(ids, gt.component) {
			ids = append(ids, gt.component)
		}
	}
	slices.Sort(ids)
	return ids
}

func sortedCopy(ids []string) []string {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	return ids
}

// TestGoFastChecksRunShortTests: verify:fast and hook:check run every Go
// component's tests, in short mode (spec 0056 Desired Behavior 2).
func TestGoFastChecksRunShortTests(t *testing.T) {
	for name, setup := range goTaskSetups {
		t.Run(name, func(t *testing.T) {
			s := setup(t)
			g := s.graph(t)
			for _, task := range []string{"verify:fast", "hook:check"} {
				tests := g.goTests(t, task)
				if got, want := components(tests), sortedCopy(s.goComponents); !slices.Equal(got, want) {
					t.Errorf("%s runs go test in %q, want %q", task, got, want)
				}
				for _, gt := range tests {
					if _, short := goFlag(gt.command, "-short"); !short {
						t.Errorf("%s runs %q in %q without -short", task, gt.command, gt.component)
					}
				}
			}
		})
	}
}

// TestGoFullTestsHaveTimeout: test, test:race and verify run every test,
// with a 30-minute timeout (spec 0056 Desired Behavior 3 and 4).
func TestGoFullTestsHaveTimeout(t *testing.T) {
	for name, setup := range goTaskSetups {
		t.Run(name, func(t *testing.T) {
			s := setup(t)
			g := s.graph(t)
			for _, task := range []string{"test", "verify"} {
				assertFullTests(t, task, g.goTests(t, task), s.goComponents)
			}
			for _, c := range s.goComponents {
				race := "test:race"
				if c != "" {
					race = c + ":test:race"
				}
				assertFullTests(t, race, g.goTests(t, race), []string{c})
			}
		})
	}
}

// assertFullTests checks that tests run in exactly the want components, none
// in short mode, each with the 30-minute timeout.
func assertFullTests(t *testing.T, what string, tests []goTest, want []string) {
	t.Helper()
	if got := components(tests); !slices.Equal(got, sortedCopy(want)) {
		t.Errorf("%s runs go test in %q, want %q", what, got, want)
	}
	for _, gt := range tests {
		if _, short := goFlag(gt.command, "-short"); short {
			t.Errorf("%s runs %q in short mode; only the fast checks may", what, gt.command)
		}
		if v, _ := goFlag(gt.command, "-timeout"); v != goTimeout {
			t.Errorf("%s runs %q with timeout %q, want %s", what, gt.command, v, goTimeout)
		}
	}
}

// TestGoPrePushRunsFullTests: the pre-push hook runs every Go component's
// full tests with the 30-minute timeout, so pushing needs no GOFLAGS (spec
// 0056 Desired Behavior 3, and the constraint that pre-push keeps running
// the full suite even where it also runs verify:fast).
func TestGoPrePushRunsFullTests(t *testing.T) {
	for name, setup := range goTaskSetups {
		t.Run(name, func(t *testing.T) {
			s := setup(t)
			g := s.graph(t)
			var hooks struct {
				PrePush struct {
					Commands map[string]struct {
						Run  string `yaml:"run"`
						Root string `yaml:"root"`
					} `yaml:"commands"`
				} `yaml:"pre-push"`
			}
			if err := yaml.Unmarshal(s.lefthook, &hooks); err != nil {
				t.Fatalf("parsing lefthook.yml: %v", err)
			}
			var full []goTest
			for cmdName, c := range hooks.PrePush.Commands {
				if c.Root != "" {
					t.Fatalf("pre-push %s runs from %s; this test reads root commands only", cmdName, filepath.ToSlash(c.Root))
				}
				// Hooks call tasks instead of duplicating command lists
				// (docs/architecture/principles.md), so the 30-minute
				// timeout lives in task test alone.
				if direct := module.GoTestCommands(c.Run); len(direct) > 0 {
					t.Errorf("pre-push %s runs %q directly; call the test task instead", cmdName, direct)
				}
				var tests []goTest
				if f := strings.Fields(c.Run); len(f) >= 2 && f[0] == "task" {
					for _, called := range f[1:] {
						if !strings.HasPrefix(called, "-") {
							tests = append(tests, g.goTests(t, called)...)
						}
					}
				}
				for _, gt := range tests {
					if _, short := goFlag(gt.command, "-short"); !short {
						full = append(full, gt)
					}
				}
			}
			assertFullTests(t, "pre-push", full, s.goComponents)
		})
	}
}
