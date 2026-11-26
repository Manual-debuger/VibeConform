package guardrule_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

func TestParseTarget(t *testing.T) {
	cases := []struct {
		tool, input string
		want        guardrule.Target
		err         bool
	}{
		{"Bash", `{"command": "ls", "description": "rm -rf"}`, guardrule.NewCommandTarget("Bash", "ls"), false},
		{"PowerShell", `{"command": "Get-Date"}`, guardrule.NewCommandTarget("PowerShell", "Get-Date"), false},
		{"Write", `{"file_path": "/r/.env", "content": "rm -rf x"}`, guardrule.NewPathTarget("Write", "/r/.env"), false},
		{"MultiEdit", `{"file_path": "a", "edits": []}`, guardrule.NewPathTarget("MultiEdit", "a"), false},
		{"NotebookEdit", `{"notebook_path": "n.ipynb"}`, guardrule.NewPathTarget("NotebookEdit", "n.ipynb"), false},
		{"Read", `{"file_path": ".env"}`, guardrule.OtherTarget("Read"), false},
		{"Bash", `{"script": "git clean -fd"}`, guardrule.Target{}, true},
		{"Bash", `{"command": 1}`, guardrule.Target{}, true},
		{"Edit", `[]`, guardrule.Target{}, true},
		{"Write", `{"file_path": ""}`, guardrule.Target{}, true},
		{"NotebookEdit", `{"file_path": "n.ipynb"}`, guardrule.Target{}, true},
	}
	for _, c := range cases {
		got, err := guardrule.ParseTarget(c.tool, json.RawMessage(c.input))
		var pe *guardrule.PayloadError
		if c.err != errors.As(err, &pe) || got != c.want {
			t.Errorf("ParseTarget(%s, %s) = %+v, %v", c.tool, c.input, got, err)
		}
	}
}

// A target says what it is by its kind, not by its tool (spec 0073,
// "Typed state"): its one text is the command or the path.
func TestTargetKind(t *testing.T) {
	cases := []struct {
		tool, input string
		kind        guardrule.TargetKind
		text        string
	}{
		{"Bash", `{"command": "ls"}`, guardrule.CommandTarget, "ls"},
		{"PowerShell", `{"command": ""}`, guardrule.CommandTarget, ""},
		{"Edit", `{"file_path": "a.go"}`, guardrule.PathTarget, "a.go"},
		{"NotebookEdit", `{"notebook_path": "n.ipynb"}`, guardrule.PathTarget, "n.ipynb"},
		{"Read", `{"file_path": "a.go"}`, guardrule.NoTarget, ""},
	}
	for _, c := range cases {
		got, err := guardrule.ParseTarget(c.tool, json.RawMessage(c.input))
		if err != nil || got.Tool() != c.tool || got.Kind() != c.kind || got.Text() != c.text {
			t.Errorf("ParseTarget(%s, %s) = (%s, %v, %q), %v; want (%s, %v, %q)",
				c.tool, c.input, got.Tool(), got.Kind(), got.Text(), err, c.tool, c.kind, c.text)
		}
	}
	if (guardrule.Target{}).Kind() != guardrule.NoTarget {
		t.Error("the zero Target has a kind")
	}
}

func TestRelPath(t *testing.T) {
	cases := []struct{ root, p, want string }{
		{"/repo", "/repo/.env", ".env"},
		{"/repo/", "/repo/a/../certs/x.pem", "certs/x.pem"},
		{"/repo", "certs/x.pem", "certs/x.pem"},
		{"/repo", "./certs/x.pem", "certs/x.pem"},
		{"/repo", "/home/u/.ssh/id_rsa", "/home/u/.ssh/id_rsa"},
		{"/repo", "/repository/.env", "/repository/.env"},
		{`D:\repo`, `d:\Repo\.claude\hooks\policy.json`, ".claude/hooks/policy.json"},
		{`D:\repo`, `C:\repo\credentials.json`, "C:/repo/credentials.json"},
		{"", "/repo/.env", "/repo/.env"},
	}
	for _, c := range cases {
		if got := guardrule.RelPath(c.root, c.p); got != c.want {
			t.Errorf("RelPath(%q, %q) = %q, want %q", c.root, c.p, got, c.want)
		}
	}
}

// TestRelPathDotDot is a regression test (spec 0069, bug 1): a path that
// leaves the root with ".." and comes back is the root's path. It was
// cleaned only after it was compared with the root, so
// "../repo/.claude/settings.json" stayed outside it.
func TestRelPathDotDot(t *testing.T) {
	cases := []struct{ root, p, want string }{
		{"/repo", "/repo/../repo/.claude/settings.json", ".claude/settings.json"},
		{"/repo", "../repo/.env", ".env"},
		{"/repo/", "a/../../repo/x", "x"},
		{"/repo/sub/..", "/repo/.env", ".env"},
		{"/repo", "../other/.env", "../other/.env"},
		{"/repo", "/repo/../other/.env", "/other/.env"},
		{"/repo", "..", ".."},
		{`D:\repo`, `..\REPO\.env`, ".env"},
		{`d:\repo`, `D:\Repo\..\repo\.claude\hooks\x.sh`, ".claude/hooks/x.sh"},
		{`D:\repo`, `..\sib\x`, "../sib/x"},
		{`D:\repo`, `D:\repo\..\sib\x`, "D:/sib/x"},
	}
	for _, c := range cases {
		if got := guardrule.RelPath(c.root, c.p); got != c.want {
			t.Errorf("RelPath(%q, %q) = %q, want %q", c.root, c.p, got, c.want)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, p string
		want       bool
	}{
		{".env", ".env", true},
		{".env", "a/.env", false},
		{"**/.env", "a/b/.env", true},
		{"**/.env", ".env", true},
		{"*.pem", "server.pem", true},
		{"*.pem", "certs/server.pem", false},
		{"certs/*.pem", "certs/server.pem", true},
		{"certs/**", "certs/a/b.pem", true},
		{"certs/**", "certs", false},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"?.env", "x.env", true},
		{"?.env", "/.env", false},
		{".env.(x)", ".env.(x)", true},
		{".ENV", ".env", false},
		{"résumé/*", "résumé/a", true},
	}
	for _, c := range cases {
		if got := guardrule.MatchGlob(c.pattern, c.p); got != c.want {
			t.Errorf("MatchGlob(%q, %q) = %v", c.pattern, c.p, got)
		}
	}
}

func TestExceptionMatches(t *testing.T) {
	const root = "/repo"
	cases := []struct {
		name string
		ex   guardrule.Exception
		t    guardrule.Target
		want bool
	}{
		{"path glob inside the root",
			exception(t, "secret-file", "**/.env.example"),
			guardrule.NewPathTarget("Write", "/repo/config/.env.example"), true},
		{"path glob does not match another file",
			exception(t, "secret-file", "**/.env.example"),
			guardrule.NewPathTarget("Write", "/repo/.env"), false},
		{"a path rule does not match a command",
			exception(t, "secret-file", "*"),
			guardrule.NewCommandTarget("Bash", "echo > .env"), false},
		{"command prefix",
			exception(t, "git-clean-fd", "git clean -fdx build/"),
			guardrule.NewCommandTarget("Bash", "git clean -fdx build/"), true},
		{"command prefix with more words",
			exception(t, "git-clean-fd", "git clean -fdx"),
			guardrule.NewCommandTarget("Bash", "git clean -fdx build/"), true},
		{"a longer match is no prefix",
			exception(t, "git-clean-fd", "git clean -fdx build/"),
			guardrule.NewCommandTarget("Bash", "git clean -fdx"), false},
		{"a word is compared whole",
			exception(t, "git-clean-fd", "git clean -fdx build"),
			guardrule.NewCommandTarget("Bash", "git clean -fdx build/"), false},
		{"in a list, behind a wrapper and quoted",
			exception(t, "git-clean-fd", "git clean -fdx build/"),
			guardrule.NewCommandTarget("Bash", `cd x && sudo "git" clean '-fdx' build/`), true},
		{"the program by its path",
			exception(t, "rm-rf", "rm -rf build"),
			guardrule.NewCommandTarget("Bash", "/bin/rm -rf build"), true},
		{"an expansion never matches",
			exception(t, "rm-rf", "rm -rf build"),
			guardrule.NewCommandTarget("Bash", "rm -rf $d"), false},
		{"a command that does not parse matches nothing",
			exception(t, "rm-rf", "rm -rf build"),
			guardrule.NewCommandTarget("Bash", "rm -rf build '"), false},
		{"PowerShell without case and by alias",
			exception(t, "remove-item-recurse-force", "Remove-Item build"),
			guardrule.NewCommandTarget("PowerShell", "ri BUILD -r -fo"), true},
		{"PowerShell parameters are compared as words",
			exception(t, "remove-item-recurse-force", "Remove-Item -Recurse"),
			guardrule.NewCommandTarget("PowerShell", "Remove-Item -Rec x"), false},
		{"a command rule does not match a file edit",
			exception(t, "rm-rf", "rm"),
			guardrule.NewPathTarget("Write", "rm"), false},
	}
	for _, c := range cases {
		if got := c.ex.Matches(c.t, root); got != c.want {
			t.Errorf("%s: Matches = %v", c.name, got)
		}
	}
}

// With the simple commands a rule refused, a command exception matches
// only those (spec 0072, "The audit's reading under 2.0.0").
func TestExceptionMatchesRefused(t *testing.T) {
	const root = "/repo"
	cases := []struct {
		name    string
		ex      guardrule.Exception
		t       guardrule.Target
		refused []string
		want    bool
	}{
		{"another simple command of the case",
			exception(t, "rm-rf", "cd /tmp"),
			guardrule.NewCommandTarget("Bash", "cd /tmp && rm -rf"), []string{"rm -rf"}, false},
		{"the refused simple command",
			exception(t, "rm-rf", "rm -rf"),
			guardrule.NewCommandTarget("Bash", "cd /tmp && rm -rf"), []string{"rm -rf"}, true},
		{"a nested refused simple command",
			exception(t, "rm-rf", "rm -rf build"),
			guardrule.NewCommandTarget("Bash", "bash -c 'rm -rf build'"), []string{"rm -rf build"}, true},
		{"the shell that runs the refused simple command",
			exception(t, "rm-rf", "bash -c"),
			guardrule.NewCommandTarget("Bash", "bash -c 'rm -rf build'"), []string{"rm -rf build"}, false},
		{"refused does not change a path match",
			exception(t, "secret-file", ".env"),
			guardrule.NewPathTarget("Write", "/repo/.env"), []string{"x"}, true},
	}
	for _, c := range cases {
		if got := c.ex.MatchesRefused(c.t, root, c.refused); got != c.want {
			t.Errorf("%s: MatchesRefused = %v", c.name, got)
		}
	}
}

func TestRuleMatches(t *testing.T) {
	p, err := guardrule.ParsePolicy([]byte(`{"version": 2, "rules": [
		{"id": "tf", "level": "deny", "reason": "x", "command": "terraform destroy"},
		{"id": "kubectl-prod", "level": "ask", "reason": "x", "command": "kubectl", "args": "^--context=prod$"},
		{"id": "lock", "level": "deny", "reason": "x", "tools": ["Write"], "path": "**/go.sum"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	tf, kube, lock := p.Rules[0], p.Rules[1], p.Rules[2]
	cmd := func(shell guardrule.Shell, src string) guardrule.Command {
		t.Helper()
		cmds, err := guardrule.Commands(shell, src)
		if err != nil {
			t.Fatal(err)
		}
		return cmds[len(cmds)-1]
	}
	cases := []struct {
		name string
		rule guardrule.Rule
		cmd  guardrule.Command
		want bool
	}{
		{"prefix", tf, cmd(guardrule.ShellBash, "terraform destroy -auto-approve"), true},
		{"other subcommand", tf, cmd(guardrule.ShellBash, "terraform plan"), false},
		{"an unknown word can be the prefix", tf, cmd(guardrule.ShellBash, "terraform $sub"), true},
		{"a quoted expansion is one operand", tf, cmd(guardrule.ShellBash, `terraform "$sub"`), true},
		{"a known word rules it out", tf, cmd(guardrule.ShellBash, "terraform plan $x"), false},
		{"args match a later word", kube, cmd(guardrule.ShellBash, "kubectl get pods --context=prod"), true},
		{"args do not match", kube, cmd(guardrule.ShellBash, "kubectl get pods --context=dev"), false},
		{"an unknown word can match args", kube, cmd(guardrule.ShellBash, "kubectl get $flags"), true},
		{"in PowerShell without case", tf, cmd(guardrule.ShellPowerShell, "Terraform DESTROY"), true},
	}
	for _, c := range cases {
		if got := c.rule.MatchesCommand(c.cmd); got != c.want {
			t.Errorf("%s: MatchesCommand = %v", c.name, got)
		}
	}
	if !lock.MatchesPath("/repo/sub/go.sum", "/repo") || lock.MatchesPath("/repo/go.mod", "/repo") {
		t.Error("path rule")
	}
	if !lock.AppliesTo("Write") || lock.AppliesTo("Edit") || !tf.AppliesTo("PowerShell") || tf.AppliesTo("Write") {
		t.Error("AppliesTo")
	}
}

// A rule with an args that does not compile cannot be built, so no deny
// rule can fail open on it (spec 0069, bug 4; spec 0073 replaces its
// fail-closed match with this refusal).
func TestRuleWithUncompilableArgsCannotBeBuilt(t *testing.T) {
	r, err := guardrule.NewRule(guardrule.RuleSpec{ID: "kubectl-bad", Level: guardrule.Deny, Reason: "x",
		Kind: guardrule.CommandRule, Pattern: "kubectl", Args: "(["})
	var fe *guardrule.FieldError
	if !errors.As(err, &fe) || fe.Field != "args" {
		t.Fatalf("NewRule = %+v, %v; want an args error", r, err)
	}
}

// The zero Rule and the zero Exception match no command and no path.
func TestZeroRuleAndExceptionMatchNothing(t *testing.T) {
	cmds, err := guardrule.Commands(guardrule.ShellBash, "rm -rf build")
	if err != nil {
		t.Fatal(err)
	}
	var r guardrule.Rule
	if r.MatchesCommand(cmds[0]) || r.MatchesPath("/repo/x", "/repo") || r.MatchesPath("", "") {
		t.Error("the zero Rule matches")
	}
	var e guardrule.Exception
	if e.MatchesCommand(cmds[0]) || e.MatchesPath("/repo/x", "/repo") || e.MatchesPath("", "") ||
		e.Matches(guardrule.NewCommandTarget("Bash", "rm -rf build"), "/repo") ||
		e.Matches(guardrule.NewPathTarget("Write", "/repo/x"), "/repo") {
		t.Error("the zero Exception matches")
	}
}

// A match that parses only as PowerShell matches a PowerShell command
// and not a Bash command with the same words.
func TestPowerShellOnlyMatch(t *testing.T) {
	const match = "Remove-Item \"a`\"b\""
	if _, err := guardrule.Commands(guardrule.ShellBash, match); err == nil {
		t.Fatalf("%s parses as Bash; the test needs a match that does not", match)
	}
	e := exception(t, "remove-item-recurse-force", match)
	if !e.Matches(guardrule.NewCommandTarget("PowerShell", "Remove-Item \"a`\"b\" -Recurse -Force"), "/repo") {
		t.Error("the match does not match the PowerShell command")
	}
	if e.Matches(guardrule.NewCommandTarget("Bash", `Remove-Item 'a"b' -Recurse -Force`), "/repo") {
		t.Error("the match matches a Bash command")
	}
	r, err := guardrule.NewRule(guardrule.RuleSpec{ID: "r", Level: guardrule.Deny, Reason: "x", Kind: guardrule.CommandRule, Pattern: match})
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := guardrule.Commands(guardrule.ShellPowerShell, "Remove-Item \"a`\"b\" -Recurse")
	bash, _ := guardrule.Commands(guardrule.ShellBash, `Remove-Item 'a"b' -Recurse`)
	if !r.MatchesCommand(ps[0]) || r.MatchesCommand(bash[0]) {
		t.Error("a PowerShell-only rule: want a match in PowerShell only")
	}
}

// A policy's command matches are parsed once per shell, and its globs
// compiled once, when the policy is read; matching parses and compiles
// nothing again (spec 0073).
func TestMatchesAreParsedOnce(t *testing.T) {
	const n, m = 3, 4 // command exceptions; simple commands of the command
	parses, compiles := guardrule.MatchParses(), guardrule.GlobCompiles()
	p, err := guardrule.ParsePolicy([]byte(`{"version": 2,
		"rules": [
			{"id": "tf", "level": "deny", "reason": "x", "command": "terraform destroy", "args": "^-auto"},
			{"id": "lock", "level": "deny", "reason": "x", "path": "**/go.sum"}
		],
		"exceptions": [
			{"rule": "rm-rf", "match": "rm -rf build", "reason": "x"},
			{"rule": "git-clean-fd", "match": "git clean -fdx", "reason": "x"},
			{"rule": "git-push-force", "match": "git push --force origin feat/", "reason": "x"},
			{"rule": "secret-file", "match": "**/.env.example", "reason": "x"}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := guardrule.MatchParses()-parses, int64(2*(n+1)); got != want {
		t.Errorf("reading the policy parsed %d matches, want %d (each command match once per shell)", got, want)
	}
	if got := guardrule.GlobCompiles() - compiles; got != 2 {
		t.Errorf("reading the policy compiled %d globs, want 2", got)
	}
	parses, compiles = guardrule.MatchParses(), guardrule.GlobCompiles()
	command := "rm -rf build && git clean -fdx && git push --force origin feat/x && terraform destroy -auto-approve"
	cmds, err := guardrule.Commands(guardrule.ShellBash, command)
	if err != nil || len(cmds) != m {
		t.Fatalf("%d simple commands, %v; want %d", len(cmds), err, m)
	}
	for _, e := range p.Exceptions {
		e.Matches(guardrule.NewCommandTarget("Bash", command), "/repo")
		e.MatchesRefused(guardrule.NewCommandTarget("Bash", command), "/repo", []string{"rm -rf build"})
		e.Matches(guardrule.NewPathTarget("Write", "/repo/a/.env.example"), "/repo")
		for _, c := range cmds {
			e.MatchesCommand(c)
		}
	}
	for _, r := range p.Rules {
		r.MatchesPath("/repo/a/go.sum", "/repo")
		for _, c := range cmds {
			r.MatchesCommand(c)
		}
	}
	if got := guardrule.MatchParses() - parses; got != 0 {
		t.Errorf("matching parsed %d matches again", got)
	}
	if got := guardrule.GlobCompiles() - compiles; got != 0 {
		t.Errorf("matching compiled %d globs again", got)
	}
}

// MatchGlob compiles a pattern it has seen once.
func TestMatchGlobCachesItsPattern(t *testing.T) {
	const pattern = "cache-test/**/x.*"
	guardrule.MatchGlob(pattern, "cache-test/x.go")
	compiles := guardrule.GlobCompiles()
	for range 5 {
		if !guardrule.MatchGlob(pattern, "cache-test/a/x.go") {
			t.Fatal("MatchGlob does not match")
		}
	}
	if got := guardrule.GlobCompiles() - compiles; got != 0 {
		t.Errorf("MatchGlob compiled the same pattern %d more times", got)
	}
}

// exception builds an exception the constructor accepts.
func exception(t *testing.T, rule, match string) guardrule.Exception {
	t.Helper()
	e, err := guardrule.NewException(rule, match, "x")
	if err != nil {
		t.Fatalf("NewException(%q, %q): %v", rule, match, err)
	}
	return e
}
