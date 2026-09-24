package module

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// hookTasks are the tasks spec 0023 adds for the agents to call. Like
// hook:guard, none has a desc (agents call them, people don't) and each is
// silent (Task would otherwise echo the script into the agent's context).
var hookTasks = []string{"hook:context", "hook:format", "hook:check", "hook:done"}

// hookScript returns a hook task's single shell command.
func hookScript(t *testing.T, tf describedTaskfile, name string) string {
	t.Helper()
	task, ok := tf.Tasks[name]
	if !ok {
		t.Fatalf("no %s task; the agent configs call it", name)
	}
	if len(task.Cmds) != 1 || task.Cmds[0].shell == "" {
		t.Fatalf("%s should be exactly one shell command, has %d cmds", name, len(task.Cmds))
	}
	return task.Cmds[0].shell
}

type silentTaskfile struct {
	Tasks map[string]struct {
		Silent bool `yaml:"silent"`
	} `yaml:"tasks"`
}

func TestHookTasksShape(t *testing.T) {
	for _, path := range repoToolingTaskfiles {
		t.Run(path, func(t *testing.T) {
			_, tf := readTaskfile(t, path)
			data, err := os.ReadFile(filepath.FromSlash(path))
			if err != nil {
				t.Fatal(err)
			}
			var silent silentTaskfile
			if err := yaml.Unmarshal(data, &silent); err != nil {
				t.Fatal(err)
			}
			for _, name := range hookTasks {
				hookScript(t, tf, name)
				if tf.Tasks[name].Desc != "" {
					t.Errorf("%s has desc %q; agents call it, people don't, so it stays out of task --list", name, tf.Tasks[name].Desc)
				}
				if !silent.Tasks[name].Silent {
					t.Errorf("%s is not silent; Task would echo the script into the agent's hook output", name)
				}
			}
			// Both run verify:fast and exit 2 on failure, the one code both
			// agents act on. -s keeps Task's own command echo out; 1>&2
			// moves the tools' output (go test prints failures on stdout) to
			// stderr, which is what an agent shows the model on exit 2.
			for _, name := range []string{"hook:check", "hook:done"} {
				if script := hookScript(t, tf, name); !strings.Contains(script, "task -s verify:fast 1>&2 || exit 2") {
					t.Errorf("%s does not run %q:\n%s", name, "task -s verify:fast 1>&2 || exit 2", script)
				}
			}
		})
	}
}

// formatExtensions is, per template, the files hook:format touches: the
// same extensions as the fmt task, so the hook never formats a file fmt
// would leave alone.
var formatExtensions = map[string][]string{
	"repotooling/templates/Taskfile.yml":   {"*.go"},
	"tsrepotooling/templates/Taskfile.yml": {"*.css", "*.js", "*.json", "*.jsx", "*.md", "*.ts", "*.tsx"},
	"pyrepotooling/templates/Taskfile.yml": {"*.py"},
}

var pathspec = regexp.MustCompile(`'(\*\.[a-z]+)'`)

// TestHookFormatScope pins spec 0023 4.2: the changed files come from git
// (tracked changes relative to the Taskfile's directory, plus untracked
// files git doesn't ignore), the list is NUL-separated so paths with spaces
// survive, an empty list runs nothing (GNU xargs would otherwise run the
// formatter with no arguments), and every tool failure is exit 2.
func TestHookFormatScope(t *testing.T) {
	for _, path := range repoToolingTaskfiles {
		t.Run(path, func(t *testing.T) {
			_, tf := readTaskfile(t, path)
			script := hookScript(t, tf, "hook:format")

			for _, want := range []string{
				"git diff -z --relative --name-only --diff-filter=d HEAD --",
				"git ls-files -z --others --exclude-standard --",
				`[ -n "$(list)" ] || exit 0`,
			} {
				if !strings.Contains(script, want) {
					t.Errorf("hook:format does not contain %q", want)
				}
			}

			selectors := 0
			for line := range strings.SplitSeq(script, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "git ") {
					continue
				}
				selectors++
				var got []string
				for _, m := range pathspec.FindAllStringSubmatch(line, -1) {
					got = append(got, m[1])
				}
				slices.Sort(got)
				if want := formatExtensions[path]; !slices.Equal(got, want) {
					t.Errorf("%q selects %v, want %v (fmt's extensions)", line, got, want)
				}
			}
			if selectors != 2 {
				t.Errorf("hook:format has %d git file selectors, want 2 (changed and untracked)", selectors)
			}

			runs := 0
			for line := range strings.SplitSeq(script, "\n") {
				line = strings.TrimSpace(line)
				if !strings.Contains(line, "xargs") {
					continue
				}
				runs++
				if !strings.Contains(line, "xargs -0 ") {
					t.Errorf("%q does not use xargs -0; the list is NUL-separated", line)
				}
				if !strings.HasSuffix(line, "|| exit 2") {
					t.Errorf("%q does not end in || exit 2", line)
				}
			}
			if runs == 0 {
				t.Error("hook:format runs no tool")
			}
		})
	}
}

// contextCommands is everything hook:context may run: git, the version
// probes, and the shell's own builtins. Spec 0023 4.1 names what session
// start must never do (install dependencies, start services, run tests,
// migrate, rebuild graphs); an allowlist catches all of those, and anything
// else, without having to name them.
var contextCommands = []string{
	"git", "go", "node", "pnpm", "uv",
	"echo", "printf", "test", "[", "read", "exit", "continue", "true",
	"if", "then", "elif", "else", "fi", "for", "do", "done", "while", "case", "esac", "EOF",
}

// commandSeparators splits a script into segments that each start with a
// command word: pipes, lists, command substitutions, and the words after
// which a new command begins.
var commandSeparators = regexp.MustCompile(`\|\||&&|[|;&\n]|\$\(|\bthen\b|\bdo\b|\belse\b`)

// commandWords returns the first word of every segment, skipping variable
// assignments and anything that can't be a command name (quoted strings,
// expansions, arithmetic, redirections, closing brackets).
func commandWords(script string) []string {
	var words []string
	for _, segment := range commandSeparators.Split(script, -1) {
		for word := range strings.FieldsSeq(segment) {
			word = strings.TrimPrefix(word, "!")
			if word == "" || strings.ContainsAny(word[:1], `"'$*()-<>0123456789}]{…`) {
				break
			}
			if name, value, isAssignment := strings.Cut(word, "="); isAssignment && name != "" {
				// A quoted value runs to the end of the segment
				// (b="detached at "); only an unquoted one (IFS=) can
				// be followed by a command.
				if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'") {
					break
				}
				continue
			}
			words = append(words, word)
			break
		}
	}
	return words
}

func TestHookContextRunsNothingHeavy(t *testing.T) {
	for _, path := range repoToolingTaskfiles {
		t.Run(path, func(t *testing.T) {
			_, tf := readTaskfile(t, path)
			script := hookScript(t, tf, "hook:context")
			for _, word := range commandWords(script) {
				if !slices.Contains(contextCommands, word) {
					t.Errorf("hook:context runs %q; session start is facts, not work (spec 0023 4.1)", word)
				}
			}
			// Probing a runtime's version is allowed; running anything
			// through it is not. uv run in particular can download a Python.
			for _, banned := range []string{"uv run", "uv sync", "pnpm install", "pnpm exec", "go run", "go test", "go build", "go mod", "task "} {
				if strings.Contains(script, banned) {
					t.Errorf("hook:context contains %q", banned)
				}
			}
		})
	}
}

// TestCommandWords keeps the allowlist test honest: a heavy command hidden
// in a substitution, a list, or after an assignment is still found.
func TestCommandWords(t *testing.T) {
	script := "x=\"$(docker compose up)\"\n[ -n \"$x\" ] || pnpm install\nif true; then make migrate; fi\nIFS= read -r line\nb=\"detached at $(git rev-parse HEAD)\""
	got := commandWords(script)
	for _, want := range []string{"docker", "pnpm", "make", "read", "git"} {
		if !slices.Contains(got, want) {
			t.Errorf("commandWords missed %q; got %v", want, got)
		}
	}
	// Words inside a quoted assignment are data, not commands.
	if slices.Contains(got, "at") {
		t.Errorf("commandWords took a word of a quoted value for a command; got %v", got)
	}
}

// taskOnPath returns the task binary, skipping when it's absent unless
// VIBE_HOOKS_E2E is set: whoever set it asked for these checks, and a
// skipped run would report green having proved nothing.
func taskOnPath(t *testing.T) string {
	t.Helper()
	task, err := exec.LookPath("task")
	if err != nil {
		if os.Getenv("VIBE_HOOKS_E2E") != "" {
			t.Fatalf("VIBE_HOOKS_E2E is set but task is not on PATH: %v", err)
		}
		t.Skip("task not on PATH; the hook-guard workflow runs this in CI")
	}
	return task
}

// hookTaskfile writes a Taskfile holding the template's hook task plus
// stub tasks into dir, so a hook can run in isolation.
func hookTaskfile(t *testing.T, dir, path, hook string, stubs map[string]any) {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tasks map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	tasks := map[string]any{hook: doc.Tasks[hook]}
	maps.Copy(tasks, stubs)
	out, err := yaml.Marshal(map[string]any{"version": "3", "tasks": tasks})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Taskfile.yml"), out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// runTask runs `task -x <name>` in dir and returns the exit code and output.
func runTask(t *testing.T, task, dir, name, stdin string) (code int, stdout, stderr string) {
	t.Helper()
	// #nosec G204 -- task and name are fixed by the calling test
	cmd := exec.Command(task, "-x", name)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("running task %s: %v", name, err)
		}
		code = exit.ExitCode()
	}
	return code, out.String(), errOut.String()
}

type stopCase struct {
	Name   string `json:"name"`
	Stdin  string `json:"stdin"`
	Active bool   `json:"active"`
}

func loadStopCorpus(t *testing.T) []stopCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "stop_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Cases []stopCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Cases) == 0 {
		t.Fatal("stop corpus has no cases")
	}
	return c.Cases
}

// TestHookDoneBlocksOnce runs each template's hook:done through Task against
// a verify:fast stub. A failing check blocks the stop (exit 2, reason on
// stderr), except when stop_hook_active says this stop was already blocked
// once: then it is let through (exit 0), still saying why (spec 0023 4.4).
// A passing check never blocks. Nothing goes to stdout in any case, since
// agents parse stdout on exit 0.
func TestHookDoneBlocksOnce(t *testing.T) {
	task := taskOnPath(t)
	for _, path := range repoToolingTaskfiles {
		for _, passing := range []bool{false, true} {
			dir := t.TempDir()
			stub := map[string]any{"cmds": []string{"echo 'verify:fast failed: TestAdd' && exit 1"}}
			if passing {
				stub = map[string]any{"cmds": []string{"echo ok"}}
			}
			hookTaskfile(t, dir, path, "hook:done", map[string]any{"verify:fast": stub})

			for _, tc := range loadStopCorpus(t) {
				t.Run(fmt.Sprintf("%s/passing=%v/%s", path, passing, tc.Name), func(t *testing.T) {
					code, stdout, stderr := runTask(t, task, dir, "hook:done", tc.Stdin)
					want := 0
					if !passing && !tc.Active {
						want = 2
					}
					if code != want {
						t.Errorf("exit %d, want %d; stderr: %s", code, want, stderr)
					}
					if !passing && !strings.Contains(stderr, "verify:fast failed") {
						t.Errorf("stderr does not carry the failure; the model would see no reason:\n%s", stderr)
					}
					if stdout != "" {
						t.Errorf("stdout = %q; agents parse stdout on exit 0", stdout)
					}
				})
			}
		}
	}
}

// gitRepo creates a git repository with one commit in dir.
func gitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"config", "core.autocrlf", "false"},
		{"add", "-A"},
		{"commit", "-q", "-m", "init"},
	} {
		// #nosec G204 -- fixed arguments
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// toolchainLines are the runtime probes each standard's context prints.
var toolchainLines = map[string][]string{
	"repotooling/templates/Taskfile.yml":   {"Go: "},
	"tsrepotooling/templates/Taskfile.yml": {"Node: ", "pnpm: "},
	"pyrepotooling/templates/Taskfile.yml": {"uv: ", "Python: "},
}

// TestHookContextOutput runs each template's hook:context in a scratch
// repository with more dirty files than the list shows, and checks every
// line spec 0023 4.1 names, the cap, and the size limit.
func TestHookContextOutput(t *testing.T) {
	task := taskOnPath(t)
	for _, path := range repoToolingTaskfiles {
		t.Run(path, func(t *testing.T) {
			dir := t.TempDir()
			hookTaskfile(t, dir, path, "hook:context", nil)
			gitRepo(t, dir)
			for i := range 25 {
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			code, stdout, stderr := runTask(t, task, dir, "hook:context", "")
			if code != 0 {
				t.Fatalf("exit %d; session start must never fail. stderr: %s", code, stderr)
			}
			want := append([]string{
				"Repo: " + filepath.Base(dir),
				"Branch: main",
				"State: dirty",
				"Task: ",
				"Dirty files: 25",
				"… and 5 more",
			}, toolchainLines[path]...)
			for _, line := range want {
				if !strings.Contains(stdout, line) {
					t.Errorf("output lacks %q:\n%s", line, stdout)
				}
			}
			if n := strings.Count(stdout, "?? f"); n != 20 {
				t.Errorf("lists %d files, want 20 (the cap)", n)
			}
			if len(stdout) >= 10000 {
				t.Errorf("output is %d characters; Claude Code truncates context at 10,000", len(stdout))
			}
		})
	}
}

// TestHookContextOutsideGit checks that session start in a directory that
// is not a git repository still succeeds and says so.
func TestHookContextOutsideGit(t *testing.T) {
	task := taskOnPath(t)
	for _, path := range repoToolingTaskfiles {
		t.Run(path, func(t *testing.T) {
			dir := t.TempDir()
			hookTaskfile(t, dir, path, "hook:context", nil)
			// Stop git from finding a repository above the temp directory.
			t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
			code, stdout, stderr := runTask(t, task, dir, "hook:context", "")
			if code != 0 {
				t.Fatalf("exit %d; stderr: %s", code, stderr)
			}
			if !strings.Contains(stdout, "not a git repository") {
				t.Errorf("output does not say it is outside git:\n%s", stdout)
			}
		})
	}
}

// TestHookFormatGo runs prod-go's hook:format in a scratch module: nothing
// runs on a clean tree, a changed file whose name has a space comes back
// formatted with its unused import removed (goimports is the cheap fix),
// and a syntax error is reported as exit 2. The TS and Python formatters
// need a synced example to run in; TestHookFormatEndToEnd covers them.
func TestHookFormatGo(t *testing.T) {
	task := taskOnPath(t)
	if _, err := exec.LookPath("goimports"); err != nil {
		if os.Getenv("VIBE_HOOKS_E2E") != "" {
			t.Fatalf("VIBE_HOOKS_E2E is set but goimports is not on PATH: %v", err)
		}
		t.Skip("goimports not on PATH")
	}
	dir := t.TempDir()
	hookTaskfile(t, dir, "repotooling/templates/Taskfile.yml", "hook:format", nil)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module scratch\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRepo(t, dir)

	if code, _, stderr := runTask(t, task, dir, "hook:format", ""); code != 0 {
		t.Fatalf("clean tree: exit %d; stderr: %s", code, stderr)
	}

	file := filepath.Join(dir, "a b.go")
	if err := os.WriteFile(file, []byte("package scratch\nimport \"fmt\"\nfunc  F(a,b int) int {return a+b}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTask(t, task, dir, "hook:format", "")
	if code != 0 {
		t.Fatalf("exit %d; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q; agents parse stdout on exit 0", stdout)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if want := "package scratch\n\nfunc F(a, b int) int { return a + b }\n"; string(got) != want {
		t.Errorf("formatted file = %q, want %q", got, want)
	}

	if err := os.WriteFile(file, []byte("package scratch\nfunc F( {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runTask(t, task, dir, "hook:format", ""); code != 2 || stderr == "" {
		t.Errorf("syntax error: exit %d with stderr %q, want 2 with the error", code, stderr)
	}
}

// formatSamples are, per standard, an unformatted file and what the
// formatter makes of it, for TestHookFormatEndToEnd.
var formatSamples = map[string]struct{ name, before, after string }{
	"go": {"zz_hook_format_e2e.go", "package main\nfunc  zz() {}\n", "package main\n\nfunc zz() {}\n"},
	// prod-ts's .prettierrc.json sets "semi": false.
	"ts": {"zz_hook_format_e2e.ts", "export const  zz = 1\n", "export const zz = 1\n"},
	"py": {"zz_hook_format_e2e.py", "zz  =  1\n", "zz = 1\n"},
}

// TestHookFormatEndToEnd runs hook:format through the real Taskfile of a
// synced repository, when VIBE_HOOKS_E2E_DIR names one and
// VIBE_HOOKS_E2E_LANG says which standard it is on. It writes one untracked
// file there and removes it afterwards. .github/workflows/hook-guard.yml
// sets both for each standard, on Linux and Windows.
func TestHookFormatEndToEnd(t *testing.T) {
	dir := os.Getenv("VIBE_HOOKS_E2E_DIR")
	if dir == "" {
		t.Skip("VIBE_HOOKS_E2E_DIR not set")
	}
	sample, ok := formatSamples[os.Getenv("VIBE_HOOKS_E2E_LANG")]
	if !ok {
		t.Fatalf("VIBE_HOOKS_E2E_LANG = %q, want go, ts, or py", os.Getenv("VIBE_HOOKS_E2E_LANG"))
	}
	task, err := exec.LookPath("task")
	if err != nil {
		t.Fatalf("VIBE_HOOKS_E2E_DIR is set but task is not on PATH: %v", err)
	}
	file := filepath.Join(dir, sample.name)
	if err := os.WriteFile(file, []byte(sample.before), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(file) })

	code, stdout, stderr := runTask(t, task, dir, "hook:format", "")
	if code != 0 {
		t.Fatalf("exit %d; stderr: %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q; agents parse stdout on exit 0", stdout)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sample.after {
		t.Errorf("formatted file = %q, want %q", got, sample.after)
	}
}
