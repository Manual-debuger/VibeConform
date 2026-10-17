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
		{"Bash", `{"command": "ls", "description": "rm -rf"}`, guardrule.Target{Tool: "Bash", Command: "ls"}, false},
		{"PowerShell", `{"command": "Get-Date"}`, guardrule.Target{Tool: "PowerShell", Command: "Get-Date"}, false},
		{"Write", `{"file_path": "/r/.env", "content": "rm -rf x"}`, guardrule.Target{Tool: "Write", Path: "/r/.env"}, false},
		{"MultiEdit", `{"file_path": "a", "edits": []}`, guardrule.Target{Tool: "MultiEdit", Path: "a"}, false},
		{"NotebookEdit", `{"notebook_path": "n.ipynb"}`, guardrule.Target{Tool: "NotebookEdit", Path: "n.ipynb"}, false},
		{"Read", `{"file_path": ".env"}`, guardrule.Target{Tool: "Read"}, false},
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
			guardrule.Exception{Rule: "secret-file", Match: "**/.env.example"},
			guardrule.Target{Tool: "Write", Path: "/repo/config/.env.example"}, true},
		{"path glob does not match another file",
			guardrule.Exception{Rule: "secret-file", Match: "**/.env.example"},
			guardrule.Target{Tool: "Write", Path: "/repo/.env"}, false},
		{"a path rule does not match a command",
			guardrule.Exception{Rule: "secret-file", Match: "*"},
			guardrule.Target{Tool: "Bash", Command: "echo > .env"}, false},
		{"command prefix",
			guardrule.Exception{Rule: "git-clean-fd", Match: "git clean -fdx build/"},
			guardrule.Target{Tool: "Bash", Command: "git clean -fdx build/"}, true},
		{"command prefix with more words",
			guardrule.Exception{Rule: "git-clean-fd", Match: "git clean -fdx"},
			guardrule.Target{Tool: "Bash", Command: "git clean -fdx build/"}, true},
		{"a longer match is no prefix",
			guardrule.Exception{Rule: "git-clean-fd", Match: "git clean -fdx build/"},
			guardrule.Target{Tool: "Bash", Command: "git clean -fdx"}, false},
		{"a word is compared whole",
			guardrule.Exception{Rule: "git-clean-fd", Match: "git clean -fdx build"},
			guardrule.Target{Tool: "Bash", Command: "git clean -fdx build/"}, false},
		{"in a list, behind a wrapper and quoted",
			guardrule.Exception{Rule: "git-clean-fd", Match: "git clean -fdx build/"},
			guardrule.Target{Tool: "Bash", Command: `cd x && sudo "git" clean '-fdx' build/`}, true},
		{"the program by its path",
			guardrule.Exception{Rule: "rm-rf", Match: "rm -rf build"},
			guardrule.Target{Tool: "Bash", Command: "/bin/rm -rf build"}, true},
		{"an expansion never matches",
			guardrule.Exception{Rule: "rm-rf", Match: "rm -rf build"},
			guardrule.Target{Tool: "Bash", Command: "rm -rf $d"}, false},
		{"a command that does not parse matches nothing",
			guardrule.Exception{Rule: "rm-rf", Match: "rm -rf build"},
			guardrule.Target{Tool: "Bash", Command: "rm -rf build '"}, false},
		{"PowerShell without case and by alias",
			guardrule.Exception{Rule: "remove-item-recurse-force", Match: "Remove-Item build"},
			guardrule.Target{Tool: "PowerShell", Command: "ri BUILD -r -fo"}, true},
		{"PowerShell parameters are compared as words",
			guardrule.Exception{Rule: "remove-item-recurse-force", Match: "Remove-Item -Recurse"},
			guardrule.Target{Tool: "PowerShell", Command: "Remove-Item -Rec x"}, false},
		{"a command rule does not match a file edit",
			guardrule.Exception{Rule: "rm-rf", Match: "rm"},
			guardrule.Target{Tool: "Write", Path: "rm"}, false},
	}
	for _, c := range cases {
		if got := c.ex.Matches(c.t, root); got != c.want {
			t.Errorf("%s: Matches = %v", c.name, got)
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
