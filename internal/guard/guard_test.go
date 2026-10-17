package guard_test

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// guardCase is a guard-only case (testdata/cases.json): what the guard
// does with input the contract does not test, such as payload, parse
// and policy errors and the policy.json v2 fields.
type guardCase struct {
	Name string `json:"name"`
	// Policy is written as policy.json; PolicyText is written as is;
	// NoPolicy leaves the file out. With none of them, policy.json is an
	// empty enforcing policy.
	Policy     json.RawMessage `json:"policy"`
	PolicyText *string         `json:"policy_text"`
	NoPolicy   bool            `json:"no_policy"`
	// Payload is marshalled to stdin; Stdin is sent as is.
	Payload json.RawMessage `json:"payload"`
	Stdin   *string         `json:"stdin"`
	// Want is allow, deny or ask; Rule the rule ID a deny or ask names.
	Want string `json:"want"`
	Rule string `json:"rule"`
	// Stderr is text the refusal must hold.
	Stderr string `json:"stderr"`
}

func loadCases(t *testing.T) []guardCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Cases []guardCase }
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc.Cases
}

func TestGuardCases(t *testing.T) {
	for _, c := range loadCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			policy := enforcePolicy
			switch {
			case c.NoPolicy:
				policy = ""
			case c.PolicyText != nil:
				policy = *c.PolicyText
			case c.Policy != nil:
				policy = string(c.Policy)
			}
			r := newRepo(t, policy)
			if c.PolicyText != nil && *c.PolicyText == "" {
				writeFile(t, filepath.Join(r.Root, ".claude", "hooks", "policy.json"), "")
			}
			stdin := string(c.Payload)
			if c.Stdin != nil {
				stdin = *c.Stdin
			}
			got := r.hook(t, stdin)
			switch c.Want {
			case "deny":
				want := "vibe-guard: refused by rule " + c.Rule + ": "
				if got.Code != 2 || !strings.HasPrefix(got.Stderr, want) || strings.Count(got.Stderr, "\n") != 1 {
					t.Errorf("exit %d, stderr %q; want exit 2 and one line starting %q", got.Code, got.Stderr, want)
				}
				if !strings.Contains(got.Stderr, c.Stderr) {
					t.Errorf("stderr %q does not hold %q", got.Stderr, c.Stderr)
				}
			case "allow":
				if got.Code != 0 || got.Stdout != "" || got.Stderr != "" {
					t.Errorf("exit %d, stdout %q, stderr %q; want a silent exit 0", got.Code, got.Stdout, got.Stderr)
				}
			case "ask":
				assertAsk(t, got, c.Rule)
			default:
				t.Fatalf("want %q", c.Want)
			}
		})
	}
}

// assertAsk checks Claude Code's ask answer: exit 0 and
// permissionDecision "ask" on stdout, naming the rule.
func assertAsk(t *testing.T, got result, rule string) {
	t.Helper()
	var out struct {
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if got.Code != 0 || got.Stderr != "" {
		t.Fatalf("exit %d, stderr %q; want exit 0 and nothing on stderr", got.Code, got.Stderr)
	}
	if err := json.Unmarshal([]byte(got.Stdout), &out); err != nil {
		t.Fatalf("stdout %q is not JSON: %v", got.Stdout, err)
	}
	h := out.HookSpecificOutput
	if h.HookEventName != "PreToolUse" || h.PermissionDecision != "ask" || !strings.Contains(h.PermissionDecisionReason, "rule "+rule+":") {
		t.Errorf("stdout %s; want a PreToolUse ask naming rule %s", got.Stdout, rule)
	}
}

// nested wraps "rm -rf build" in n levels of bash -c.
func nested(n int) string {
	cmd := "rm -rf build"
	for range n {
		cmd = "bash -c " + strconv.Quote(cmd)
	}
	return cmd
}

func bashPayload(command string) string {
	data, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": command}})
	return string(data)
}

// TestNestingDepth: strings run by nested shells are parsed again up to
// a depth of 8; deeper nesting is refused as a parse error.
func TestNestingDepth(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	if got := r.hook(t, bashPayload(nested(8))); got.Code != 2 || !strings.HasPrefix(got.Stderr, "vibe-guard: refused by rule rm-rf: ") {
		t.Errorf("depth 8: exit %d, stderr %q; want rm-rf", got.Code, got.Stderr)
	}
	got := r.hook(t, bashPayload(nested(9)))
	if got.Code != 2 || !strings.HasPrefix(got.Stderr, "vibe-guard: refused by rule parse-error: ") ||
		!strings.Contains(got.Stderr, "deeper than 8") {
		t.Errorf("depth 9: exit %d, stderr %q; want parse-error naming the depth", got.Code, got.Stderr)
	}
}

// TestObserveMode: a call that would be refused exits 0, says so on
// stderr and appends the same line to vibe-guard.log in the common Git
// directory. Allowed calls are not logged.
func TestObserveMode(t *testing.T) {
	r := newRepo(t, `{"version": 2, "mode": "observe"}`)
	if got := r.hook(t, bashPayload("ls")); got.Code != 0 || got.Stderr != "" {
		t.Fatalf("allowed call: exit %d, stderr %q", got.Code, got.Stderr)
	}
	got := r.hook(t, bashPayload("rm -rf build"))
	want := "vibe-guard (observe): would refuse by rule rm-rf: "
	if got.Code != 0 || !strings.HasPrefix(got.Stderr, want) {
		t.Fatalf("exit %d, stderr %q; want exit 0 and %q", got.Code, got.Stderr, want)
	}
	second := r.hook(t, `not json`)
	if second.Code != 0 || !strings.HasPrefix(second.Stderr, "vibe-guard (observe): would refuse by rule payload-error: ") {
		t.Fatalf("payload error: exit %d, stderr %q", second.Code, second.Stderr)
	}
	log, err := os.ReadFile(filepath.Join(r.CommonDir, "vibe-guard.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(log) != got.Stderr+second.Stderr {
		t.Errorf("vibe-guard.log is\n%q\nwant\n%q", log, got.Stderr+second.Stderr)
	}
	ex := r.explain(t, "rm -rf build")
	if ex.Code != 0 || !hasLine(ex.Stdout, "rule: rm-rf") || !strings.Contains(ex.Stdout, "observe") {
		t.Errorf("explain: exit %d, output\n%s\nwant exit 0, rm-rf and observe", ex.Code, ex.Stdout)
	}
}

// TestObserveModeLogsInTheCommonGitDir: from a linked worktree (a .git
// file and a commondir file), the log goes to the common directory.
func TestObserveModeLogsInTheCommonGitDir(t *testing.T) {
	main := newRepo(t, "")
	wtGitDir := filepath.Join(main.CommonDir, "worktrees", "wt")
	writeFile(t, filepath.Join(wtGitDir, "commondir"), "../..\n")
	wt := t.TempDir()
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: "+filepath.ToSlash(wtGitDir)+"\n")
	writeFile(t, filepath.Join(wt, ".claude", "hooks", "policy.json"), `{"version": 2, "mode": "observe"}`)
	sub := filepath.Join(wt, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// No CLAUDE_PROJECT_DIR: the root is found from the payload's cwd.
	payload := `{"tool_name":"Bash","tool_input":{"command":"git reset --hard"},"cwd":` + strconv.Quote(sub) + `}`
	got := runGuard(t, t.TempDir(), payload, nil, "hook")
	if got.Code != 0 || !strings.Contains(got.Stderr, "rule git-reset-hard") {
		t.Fatalf("exit %d, stderr %q", got.Code, got.Stderr)
	}
	log, err := os.ReadFile(filepath.Join(main.CommonDir, "vibe-guard.log"))
	if err != nil || string(log) != got.Stderr {
		t.Errorf("log %q (%v); want %q", log, err, got.Stderr)
	}
}

// TestRelock deletes the unlock marker in the common Git directory, and
// succeeds when there is none. It does not read stdin, which the Stop
// hook passes on to hook:done.
func TestRelock(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	marker := filepath.Join(r.CommonDir, "vibe-unlock")
	writeFile(t, marker, "2099-01-01T00:00:00Z\n")
	env := []string{"CLAUDE_PROJECT_DIR=" + r.Root}
	if got := runGuard(t, r.Root, `{"stop_hook_active": false}`, env, "relock"); got.Code != 0 || got.Stderr != "" {
		t.Fatalf("exit %d, stderr %q", got.Code, got.Stderr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("the marker is still there: %v", err)
	}
	if got := runGuard(t, r.Root, "", env, "relock"); got.Code != 0 {
		t.Errorf("relock with no marker: exit %d, stderr %q", got.Code, got.Stderr)
	}
	// Without CLAUDE_PROJECT_DIR, from a subdirectory.
	writeFile(t, marker, "2099-01-01T00:00:00Z\n")
	sub := filepath.Join(r.Root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := runGuard(t, sub, "", nil, "relock"); got.Code != 0 {
		t.Fatalf("exit %d, stderr %q", got.Code, got.Stderr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("relock from a subdirectory left the marker: %v", err)
	}
}

func TestVersion(t *testing.T) {
	got := runGuard(t, t.TempDir(), "", nil, "version")
	if got.Code != 0 || !strings.HasPrefix(got.Stdout, "vibe-guard ") {
		t.Errorf("exit %d, stdout %q", got.Code, got.Stdout)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"nonsense"}, {"hook", "extra"}, {"explain"}, {"explain", "a", "b"}, {"explain", "--tool", "Read", "x"}} {
		if got := runGuard(t, t.TempDir(), "", nil, args...); got.Code != 64 || got.Stderr == "" {
			t.Errorf("%q: exit %d, stderr %q; want a usage error", args, got.Code, got.Stderr)
		}
	}
}

// TestExplain shows the matched simple command, its words and how it was
// found; a lifted rule names its exception.
func TestExplain(t *testing.T) {
	r := newRepo(t, `{"version": 2, "exceptions": [{"rule": "rm-rf", "match": "rm -rf build", "reason": "build is generated"}]}`)
	got := r.explain(t, "sudo bash -c 'rm -r -f dist'")
	for _, want := range []string{"decision: deny", "rule: rm-rf", "  matched simple command (Bash): rm -r -f dist",
		"  words: [rm] [-r] [-f] [dist]", "  found through: sudo, bash -c"} {
		if !hasLine(got.Stdout, want) {
			t.Errorf("explain output\n%s\nhas no line %q", got.Stdout, want)
		}
	}
	if got.Code != 2 {
		t.Errorf("exit %d; want 2", got.Code)
	}
	lifted := r.explain(t, "rm -rf build")
	if lifted.Code != 0 || !hasLine(lifted.Stdout, "decision: allow") ||
		!strings.Contains(lifted.Stdout, `lifted: rule rm-rf by the exception {rule: rm-rf, match: "rm -rf build", reason: "build is generated"}`) {
		t.Errorf("exit %d, output\n%s", lifted.Code, lifted.Stdout)
	}
}

// TestExplainDefaultTool: Bash, or Write when the argument has no spaces
// and names an existing path.
func TestExplainDefaultTool(t *testing.T) {
	r := newRepo(t, enforcePolicy)
	writeFile(t, filepath.Join(r.Root, ".env"), "X=1\n")
	got := r.explain(t, ".env")
	if got.Code != 2 || !hasLine(got.Stdout, "tool: Write") || !hasLine(got.Stdout, "rule: secret-file") {
		t.Errorf("existing path: exit %d, output\n%s", got.Code, got.Stdout)
	}
	got = r.explain(t, "rm -rf .env")
	if got.Code != 2 || !hasLine(got.Stdout, "tool: Bash") || !hasLine(got.Stdout, "rule: rm-rf") {
		t.Errorf("command: exit %d, output\n%s", got.Code, got.Stdout)
	}
	got = r.explain(t, "missing.env")
	if got.Code != 0 || !hasLine(got.Stdout, "tool: Bash") {
		t.Errorf("a path that does not exist is a command: exit %d, output\n%s", got.Code, got.Stdout)
	}
}

// TestImportsNoContractOrOldPackage: internal/guard imports
// internal/guardrule and nothing else of this module (spec 0053,
// acceptance criterion 10). Tests may import internal/contract for the
// corpus.
func TestImportsNoContractOrOldPackage(t *testing.T) {
	const internal = "github.com/Manual-debuger/VibeConform/internal/"
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatal("no Go files", err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, internal) && p != internal+"guardrule" {
				t.Errorf("%s imports %s", path, p)
			}
		}
	}
}
