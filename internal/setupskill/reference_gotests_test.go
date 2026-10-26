package setupskill_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Spec 0056 Desired Behavior 2 and 3, for every reference Taskfile that
// runs go test: the fast check (verify:fast) runs the tests in short mode,
// and the full runs (test, test:race, verify) run every test with an
// explicit 30-minute timeout. Since spec 0057 the pre-push hook runs
// verify:fast, so its tests are the short ones. Moved here from
// internal/module by spec 0059 (Desired Behavior 13, Seam 2).

// goTimeout is the timeout every full go test run passes. Go's default is
// ten minutes, which this repository's suite outgrew (#125).
const goTimeout = "30m"

// goTaskSetup is one reference layout whose Taskfiles run go test: a root
// Taskfile, the Taskfiles it includes by id, and its lefthook.yml.
// goComponents are the include ids that are Go components; a
// single-language root is the one Go "component", with id "".
type goTaskSetup struct {
	root         []byte
	includes     map[string][]byte
	lefthook     []byte
	goComponents []string
}

// goTaskSetups are every Go setup the setup skill hands out.
var goTaskSetups = map[string]func(t *testing.T) goTaskSetup{
	"go": func(t *testing.T) goTaskSetup {
		t.Helper()
		return goTaskSetup{
			root:         readRef(t, "reference/taskrunner/go/Taskfile.yml"),
			lefthook:     readRef(t, "reference/hooks/go/lefthook.yml"),
			goComponents: []string{""},
		}
	},
	"monorepo": func(t *testing.T) goTaskSetup {
		t.Helper()
		dir := "reference/taskrunner/monorepo/"
		return goTaskSetup{
			root: readRef(t, dir+"Taskfile.yml"),
			includes: map[string][]byte{
				"api":    readRef(t, dir+"go.Taskfile.yml"),
				"web":    readRef(t, dir+"typescript.Taskfile.yml"),
				"worker": readRef(t, dir+"python.Taskfile.yml"),
			},
			lefthook:     readRef(t, "reference/hooks/monorepo/lefthook.yml"),
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
		for _, cmd := range goTestCommands(s.shell) {
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

// goTestCommands returns each go test invocation in a shell script: "go
// test" and its arguments, up to the end of its simple command. It finds
// one inside a command substitution, as in test:short's filter
// ("out=$(go test -short ./... 2>&1) || rc=$?"). A closing parenthesis, a
// ";" or a control operator ends the invocation, redirections are left
// out, and a comment is not read.
func goTestCommands(script string) []string {
	var cmds []string
	for line := range strings.Lines(script) {
		fields := strings.Fields(line)
		for i := 0; i+1 < len(fields); i++ {
			word := fields[i]
			if strings.HasPrefix(word, "#") {
				break
			}
			if j := strings.LastIndex(word, "$("); j >= 0 {
				word = word[j+2:]
			}
			if word != "go" || fields[i+1] != "test" {
				continue
			}
			cmd := []string{"go", "test"}
			for _, arg := range fields[i+2:] {
				if slices.Contains([]string{"|", "||", "&&", ";", "&"}, arg) || strings.HasPrefix(arg, "#") {
					break
				}
				arg, closed := strings.CutSuffix(arg, ")")
				arg, ended := strings.CutSuffix(arg, ";")
				if arg != "" && !shellRedirection.MatchString(arg) {
					cmd = append(cmd, arg)
				}
				if closed || ended {
					break
				}
			}
			cmds = append(cmds, strings.Join(cmd, " "))
		}
	}
	return cmds
}

// shellRedirection matches a redirection word such as "2>&1" or ">out".
var shellRedirection = regexp.MustCompile(`^[0-9]*[<>]`)

func TestGoTestCommands(t *testing.T) {
	for script, want := range map[string][]string{
		"go test -short ./...":                                        {"go test -short ./..."},
		"go test -timeout 30m ./...\n":                                {"go test -timeout 30m ./..."},
		"rc=0\nout=$(go test -short ./... 2>&1) || rc=$?\nexit $rc\n": {"go test -short ./..."},
		"go test ./a; go test -race ./b":                              {"go test ./a", "go test -race ./b"},
		"go test -json ./... | tee out.json":                          {"go test -json ./..."},
		"# go test ./...\necho go vet":                                nil,
		"go build ./...":                                              nil,
	} {
		if got := goTestCommands(script); !slices.Equal(got, want) {
			t.Errorf("goTestCommands(%q) = %q, want %q", script, got, want)
		}
	}
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

// TestGoFastChecksRunShortTests: verify:fast runs every Go component's
// tests, in short mode (spec 0056 Desired Behavior 2), and hook:check runs
// none (spec 0057 Desired Behavior 1).
func TestGoFastChecksRunShortTests(t *testing.T) {
	for name, setup := range goTaskSetups {
		t.Run(name, func(t *testing.T) {
			s := setup(t)
			g := s.graph(t)
			tests := g.goTests(t, "verify:fast")
			if got, want := components(tests), sortedCopy(s.goComponents); !slices.Equal(got, want) {
				t.Errorf("verify:fast runs go test in %q, want %q", got, want)
			}
			for _, gt := range tests {
				if _, short := goFlag(gt.command, "-short"); !short {
					t.Errorf("verify:fast runs %q in %q without -short", gt.command, gt.component)
				}
			}
			if tests := g.goTests(t, "hook:check"); len(tests) > 0 {
				t.Errorf("hook:check runs go test: %q; the Stop gate runs the tests", tests)
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

// TestGoPrePushRunsShortTests: the pre-push hook runs every Go component's
// tests in short mode, through verify:fast, and no full run: the full
// suite is CI's (spec 0057 Desired Behavior 9 and 10, which replace spec
// 0056 Desired Behavior 3's full pre-push run).
func TestGoPrePushRunsShortTests(t *testing.T) {
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
			var tests []goTest
			for cmdName, c := range hooks.PrePush.Commands {
				if c.Root != "" {
					t.Fatalf("pre-push %s runs from %s; this test reads root commands only", cmdName, c.Root)
				}
				// Hooks call tasks instead of duplicating command lists
				// (docs/architecture/principles.md), so the 30-minute
				// timeout lives in task test alone.
				if direct := goTestCommands(c.Run); len(direct) > 0 {
					t.Errorf("pre-push %s runs %q directly; call the test task instead", cmdName, direct)
				}
				// The run is a list such as "unset …; task verify:fast".
				for _, simple := range strings.FieldsFunc(c.Run, func(r rune) bool { return r == ';' || r == '&' || r == '|' }) {
					f := strings.Fields(simple)
					if len(f) < 2 || f[0] != "task" {
						continue
					}
					for _, called := range f[1:] {
						if !strings.HasPrefix(called, "-") {
							tests = append(tests, g.goTests(t, called)...)
						}
					}
				}
			}
			if got, want := components(tests), sortedCopy(s.goComponents); !slices.Equal(got, want) {
				t.Errorf("pre-push runs go test in %q, want %q", got, want)
			}
			for _, gt := range tests {
				if _, short := goFlag(gt.command, "-short"); !short {
					t.Errorf("pre-push runs %q in %q without -short; the full suite is CI's", gt.command, gt.component)
				}
			}
		})
	}
}
