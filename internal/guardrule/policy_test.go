package guardrule_test

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

func TestContractIDs(t *testing.T) {
	want := []string{
		"rm-rf", "remove-item-recurse-force", "git-reset-hard", "git-clean-fd", "git-push-force",
		"git-branch-force-delete", "git-discard-worktree", "secret-file", "guard-policy-file", "guardrail-settings",
		"git-skip-hooks",
	}
	if strings.Join(guardrule.IDs, ",") != strings.Join(want, ",") {
		t.Fatalf("IDs = %v, want %v", guardrule.IDs, want)
	}
	for _, id := range want {
		if !guardrule.IsContractID(id) {
			t.Errorf("IsContractID(%q) = false", id)
		}
	}
	for _, id := range []string{"parse-error", "payload-error", "policy-error", "dynamic-command"} {
		if guardrule.IsContractID(id) {
			t.Errorf("IsContractID(%q) = true for a guard-internal ID", id)
		}
		if !guardrule.IsInternalID(id) {
			t.Errorf("IsInternalID(%q) = false", id)
		}
	}
	paths := map[string]bool{"secret-file": true, "guard-policy-file": true, "guardrail-settings": true}
	for _, id := range want {
		if guardrule.IsPathRule(id) != paths[id] {
			t.Errorf("IsPathRule(%q) = %v", id, !paths[id])
		}
	}
}

func TestParsePolicyAccepts(t *testing.T) {
	p, err := guardrule.ParsePolicy([]byte(`{
		"version": 2,
		"mode": "observe",
		"rules": [
			{"id": "no-terraform-destroy", "level": "deny", "reason": "ask ops", "command": "terraform destroy"},
			{"id": "ask-kubectl", "level": "ask", "tools": ["Bash"], "reason": "prod", "command": "kubectl", "args": "^--context=prod"},
			{"id": "no-lockfile-edit", "level": "deny", "tools": ["Write", "Edit"], "reason": "use the package manager", "path": "**/go.sum"}
		],
		"exceptions": [
			{"rule": "git-clean-fd", "match": "git clean -fdx build/", "reason": "build output"},
			{"rule": "secret-file", "match": "**/.env.example", "reason": "a template"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 2 || p.Mode != guardrule.Observe || len(p.Rules) != 3 || len(p.Exceptions) != 2 {
		t.Fatalf("policy = %+v", p)
	}
	rule := func(r guardrule.Rule) string {
		return fmt.Sprintf("%s %s %s %s %q %s %q", r.ID(), r.Level(), r.Kind(), r.Pattern(), r.Args(), r.Reason(), r.Tools())
	}
	if got, want := rule(p.Rules[1]), `ask-kubectl ask command kubectl "^--context=prod" prod ["Bash"]`; got != want {
		t.Errorf("rule = %s, want %s", got, want)
	}
	if got, want := rule(p.Rules[2]), `no-lockfile-edit deny path **/go.sum "" use the package manager ["Write" "Edit"]`; got != want {
		t.Errorf("rule = %s, want %s", got, want)
	}
	if e := p.Exceptions[0]; e.Rule() != "git-clean-fd" || e.Match() != "git clean -fdx build/" || e.Reason() != "build output" {
		t.Errorf("exception = %+v", e)
	}
}

// The rule and exception constructors refuse every invalid state that
// ParsePolicy refuses, with the message ParsePolicy gives after the key
// (spec 0073, "Rules and exceptions are built valid").
func TestConstructorsRefuseWithParsePolicysMessage(t *testing.T) {
	cmd := func(s guardrule.RuleSpec) guardrule.RuleSpec {
		s.ID, s.Level, s.Reason = cmp.Or(s.ID, "r"), cmp.Or(s.Level, guardrule.Deny), cmp.Or(s.Reason, "x")
		if s.Kind == 0 {
			s.Kind, s.Pattern = guardrule.CommandRule, cmp.Or(s.Pattern, "x")
		}
		return s
	}
	const okRule = `"id": "r", "level": "deny", "reason": "x", `
	rules := []struct {
		name  string
		spec  guardrule.RuleSpec
		json  string // the rule's fields in policy.json
		field string
		msg   string
	}{
		{"a contract id", cmd(guardrule.RuleSpec{ID: "rm-rf"}), `"id": "rm-rf", "level": "deny", "reason": "x", "command": "x"`,
			"id", `"rm-rf" is a built-in guard rule ID; a repository rule needs its own`},
		{"an internal id", cmd(guardrule.RuleSpec{ID: "parse-error"}), `"id": "parse-error", "level": "deny", "reason": "x", "command": "x"`,
			"id", `"parse-error" is a built-in guard rule ID; a repository rule needs its own`},
		{"a bad level", cmd(guardrule.RuleSpec{Level: "block"}), `"id": "r", "level": "block", "reason": "x", "command": "x"`,
			"level", `is "block"; it must be "deny" or "ask"`},
		{"neither command nor path", guardrule.RuleSpec{ID: "r", Level: guardrule.Deny, Reason: "x"}, okRule[:len(okRule)-2],
			"", "must have exactly one of command and path"},
		{"a command with two commands", cmd(guardrule.RuleSpec{Pattern: "a; b"}), okRule + `"command": "a; b"`,
			"command", "does not parse as a command: is not one simple command"},
		{"a command with an expansion", cmd(guardrule.RuleSpec{Pattern: "rm $x"}), okRule + `"command": "rm $x"`,
			"command", "does not parse as a command: holds an expansion; a match is compared word for word"},
		{"args on a path rule", guardrule.RuleSpec{ID: "r", Level: guardrule.Deny, Reason: "x", Kind: guardrule.PathRule, Pattern: "x", Args: "y"},
			okRule + `"path": "x", "args": "y"`, "args", "is only allowed with command"},
		{"a bad regexp", cmd(guardrule.RuleSpec{Args: "("}), okRule + `"command": "x", "args": "("`,
			"args", "is not a regular expression: error parsing regexp: missing closing ): `(`"},
		{"an unknown tool", cmd(guardrule.RuleSpec{Tools: []string{"bash"}}), okRule + `"command": "x", "tools": ["bash"]`,
			"tools", `names "bash", which is not one of Bash, PowerShell, Write, Edit, MultiEdit, NotebookEdit`},
		{"a command rule for a file-edit tool", cmd(guardrule.RuleSpec{Tools: []string{"Write"}}), okRule + `"command": "x", "tools": ["Write"]`,
			"tools", `names "Write", which runs no command, on a command rule`},
		{"a path rule for a command tool", guardrule.RuleSpec{ID: "r", Level: guardrule.Deny, Reason: "x", Kind: guardrule.PathRule, Pattern: "x", Tools: []string{"Bash"}},
			okRule + `"path": "x", "tools": ["Bash"]`, "tools", `names "Bash", which edits no file, on a path rule`},
	}
	for _, c := range rules {
		t.Run("rule with "+c.name, func(t *testing.T) {
			r, err := guardrule.NewRule(c.spec)
			var fe *guardrule.FieldError
			if !errors.As(err, &fe) || fe.Field != c.field || fe.Msg != c.msg {
				t.Errorf("NewRule = %+v, %v; want %s %q", r, err, c.field, c.msg)
			}
			key := strings.TrimSuffix("rules[0]."+c.field, ".")
			wantParse := "policy.json: " + key + " " + c.msg
			if _, err := guardrule.ParsePolicy([]byte(`{"version": 2, "rules": [{` + c.json + `}]}`)); err == nil || err.Error() != wantParse {
				t.Errorf("ParsePolicy error = %v, want %q", err, wantParse)
			}
		})
	}
	exceptions := []struct {
		name, rule, match, field, msg string
	}{
		{"an unknown rule", "rm-r", "rm -rf x", "rule", `"rm-r" is not a guard rule ID; it must be one of ` + strings.Join(guardrule.IDs, ", ")},
		{"a repository rule", "r", "x", "rule", `"r" is not a guard rule ID; it must be one of ` + strings.Join(guardrule.IDs, ", ")},
		{"a match that does not parse", "rm-rf", "rm 'x", "match", "does not parse as a command: does not parse as Bash: 1:4: reached EOF without closing quote `'`"},
		{"a match with an expansion", "rm-rf", "rm -rf $x", "match", "does not parse as a command: holds an expansion; a match is compared word for word"},
	}
	for _, c := range exceptions {
		t.Run("exception with "+c.name, func(t *testing.T) {
			e, err := guardrule.NewException(c.rule, c.match, "x")
			var fe *guardrule.FieldError
			if !errors.As(err, &fe) || fe.Field != c.field || fe.Msg != c.msg {
				t.Errorf("NewException = %+v, %v; want %s %q", e, err, c.field, c.msg)
			}
			ex, _ := json.Marshal(map[string]string{"rule": c.rule, "match": c.match, "reason": "x"})
			wantParse := "policy.json: exceptions[0]." + c.field + " " + c.msg
			if _, err := guardrule.ParsePolicy([]byte(`{"version": 2, "exceptions": [` + string(ex) + `]}`)); err == nil || err.Error() != wantParse {
				t.Errorf("ParsePolicy error = %v, want %q", err, wantParse)
			}
		})
	}
}

// A rule or an exception with several faults is refused for the first
// in the order policy.json's keys are read, whether the JSON or the
// constructor finds it.
func TestParsePolicyNamesTheFirstFault(t *testing.T) {
	cases := []struct{ entry, want string }{
		{`"rules": [{"id": "r", "level": "block", "command": "x"}]`, `rules[0].level is "block"; it must be "deny" or "ask"`},
		{`"rules": [{"id": "rm-rf", "level": 1, "reason": "x", "command": "x"}]`, `rules[0].id "rm-rf" is a built-in guard rule ID; a repository rule needs its own`},
		{`"rules": [{"id": "r", "level": 1, "reason": "x", "command": "x"}]`, `rules[0].level is 1; it must be a string`},
		{`"rules": [{"id": "r", "level": "deny", "reason": "x", "path": "x", "args": ""}]`, `rules[0].args is only allowed with command`},
		{`"rules": [{"id": "r", "level": "deny", "reason": "x", "command": "a; b", "args": 1}]`, `rules[0].command does not parse as a command: is not one simple command`},
		{`"rules": [{"id": "r", "level": "deny", "reason": "x", "command": "x", "args": "(", "tools": "Bash"}]`, "rules[0].args is not a regular expression: error parsing regexp: missing closing ): `(`"},
		{`"rules": [{"id": "r", "level": "deny", "reason": "x", "command": "x", "args": 1, "tools": ["Write"]}]`, `rules[0].args is 1; it must be a string`},
		{`"rules": [{"id": "r", "level": "deny", "command": "x", "path": "y"}]`, `rules[0].reason is missing`},
		{`"rules": [{"id": "r", "level": "deny", "reason": "x", "command": "x", "path": 1}]`, `rules[0] must have exactly one of command and path`},
		{`"exceptions": [{"rule": "rm-r"}]`, `exceptions[0].rule "rm-r" is not a guard rule ID; it must be one of ` + strings.Join(guardrule.IDs, ", ")},
		{`"exceptions": [{"rule": "rm-rf", "match": "rm $x"}]`, `exceptions[0].match does not parse as a command: holds an expansion; a match is compared word for word`},
		{`"exceptions": [{"match": "rm $x"}]`, `exceptions[0].rule is missing`},
	}
	for _, c := range cases {
		_, err := guardrule.ParsePolicy([]byte(`{"version": 2, ` + c.entry + `}`))
		if want := "policy.json: " + c.want; err == nil || err.Error() != want {
			t.Errorf("%s: error %v, want %q", c.entry, err, want)
		}
	}
}

// A rule and an exception the constructors accept are what ParsePolicy
// builds from the same values.
func TestConstructorsAccept(t *testing.T) {
	r, err := guardrule.NewRule(guardrule.RuleSpec{ID: "kubectl-prod", Level: guardrule.Ask, Reason: "prod",
		Kind: guardrule.CommandRule, Pattern: "kubectl", Args: "^--context=prod$"})
	if err != nil {
		t.Fatal(err)
	}
	cmds, err := guardrule.Commands(guardrule.ShellBash, "kubectl get pods --context=prod")
	if err != nil {
		t.Fatal(err)
	}
	if !r.MatchesCommand(cmds[0]) || !r.AppliesTo("PowerShell") || r.AppliesTo("Write") {
		t.Errorf("NewRule built %+v", r)
	}
	e, err := guardrule.NewException("secret-file", "**/.env.example", "")
	if err != nil {
		t.Fatal(err)
	}
	if !e.MatchesPath("/repo/a/.env.example", "/repo") {
		t.Errorf("NewException built %+v", e)
	}
}

// A path exception's empty match and a rule's empty fields are refused,
// as ParsePolicy refuses them.
func TestConstructorsRefuseEmptyValues(t *testing.T) {
	if _, err := guardrule.NewException("secret-file", " ", "x"); err == nil || err.Error() != "match is empty" {
		t.Errorf("NewException with an empty match: %v", err)
	}
	if _, err := guardrule.NewRule(guardrule.RuleSpec{Level: guardrule.Deny, Reason: "x", Kind: guardrule.PathRule, Pattern: "x"}); err == nil || err.Error() != "id is empty" {
		t.Errorf("NewRule without an ID: %v", err)
	}
	if _, err := guardrule.NewRule(guardrule.RuleSpec{ID: "r", Level: guardrule.Deny, Kind: guardrule.PathRule, Pattern: "x"}); err == nil || err.Error() != "reason is empty" {
		t.Errorf("NewRule without a reason: %v", err)
	}
	if _, err := guardrule.NewRule(guardrule.RuleSpec{ID: "r", Level: guardrule.Deny, Reason: "x", Kind: guardrule.PathRule}); err == nil || err.Error() != "path is empty" {
		t.Errorf("NewRule without a path: %v", err)
	}
}

func TestParsePolicyDefaults(t *testing.T) {
	p, err := guardrule.ParsePolicy([]byte(`{"version": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != guardrule.Enforce || len(p.Rules) != 0 || len(p.Exceptions) != 0 {
		t.Fatalf("policy = %+v", p)
	}
}

// Every unknown key and invalid value is an error that names the key, so
// a typo cannot silently turn a rule off (spec 0053, "policy.json version 2").
func TestParsePolicyRejects(t *testing.T) {
	rule := func(fields string) string {
		return `{"version": 2, "rules": [{` + fields + `}]}`
	}
	exc := func(fields string) string {
		return `{"version": 2, "exceptions": [{` + fields + `}]}`
	}
	const okRule = `"id": "r", "level": "deny", "reason": "x", `
	cases := []struct {
		name, json, wantKey string
	}{
		{"not JSON", `{`, ""},
		{"not an object", `[]`, ""},
		{"no version", `{}`, "version"},
		{"version 1", `{"version": 1}`, "version"},
		{"version as a string", `{"version": "2"}`, "version"},
		{"unknown top-level key", `{"version": 2, "exception": []}`, "exception"},
		{"unknown mode", `{"version": 2, "mode": "warn"}`, "mode"},
		{"null mode", `{"version": 2, "mode": null}`, "mode"},
		{"rules not a list", `{"version": 2, "rules": {}}`, "rules"},
		{"rule not an object", `{"version": 2, "rules": [1]}`, "rules[0]"},
		{"rule unknown key", rule(okRule + `"command": "x", "comand": "y"`), "rules[0].comand"},
		{"rule without id", rule(`"level": "deny", "reason": "x", "command": "x"`), "rules[0].id"},
		{"rule with a contract id", rule(`"id": "rm-rf", "level": "deny", "reason": "x", "command": "x"`), "rules[0].id"},
		{"rule with an internal id", rule(`"id": "parse-error", "level": "deny", "reason": "x", "command": "x"`), "rules[0].id"},
		{"rule with a bad level", rule(`"id": "r", "level": "block", "reason": "x", "command": "x"`), "rules[0].level"},
		{"rule without level", rule(`"id": "r", "reason": "x", "command": "x"`), "rules[0].level"},
		{"rule without reason", rule(`"id": "r", "level": "deny", "command": "x"`), "rules[0].reason"},
		{"rule with neither command nor path", rule(`"id": "r", "level": "deny", "reason": "x"`), "rules[0]"},
		{"rule with both command and path", rule(okRule + `"command": "x", "path": "y"`), "rules[0]"},
		{"rule with an empty command", rule(okRule + `"command": " "`), "rules[0].command"},
		{"rule with args on a path rule", rule(okRule + `"path": "x", "args": "y"`), "rules[0].args"},
		{"rule with a bad regexp", rule(okRule + `"command": "x", "args": "("`), "rules[0].args"},
		{"rule with an unknown tool", rule(okRule + `"command": "x", "tools": ["bash"]`), "rules[0].tools"},
		{"command rule for a file-edit tool", rule(okRule + `"command": "x", "tools": ["Write"]`), "rules[0].tools"},
		{"path rule for a command tool", rule(okRule + `"path": "x", "tools": ["Bash"]`), "rules[0].tools"},
		{"tools not a list", rule(okRule + `"command": "x", "tools": "Bash"`), "rules[0].tools"},
		{"duplicate rule id", `{"version": 2, "rules": [{` + okRule + `"command": "x"}, {` + okRule + `"command": "y"}]}`, "rules[1].id"},
		{"exceptions not a list", `{"version": 2, "exceptions": null}`, "exceptions"},
		{"exception unknown key", exc(`"rule": "rm-rf", "match": "rm -rf x", "reason": "x", "pattern": "y"`), "exceptions[0].pattern"},
		{"exception for an unknown rule", exc(`"rule": "rm-r", "match": "rm -rf x", "reason": "x"`), "exceptions[0].rule"},
		{"exception for a repository rule", `{"version": 2, "rules": [{` + okRule + `"command": "x"}], "exceptions": [{"rule": "r", "match": "x", "reason": "x"}]}`, "exceptions[0].rule"},
		{"exception without match", exc(`"rule": "rm-rf", "reason": "x"`), "exceptions[0].match"},
		{"exception without reason", exc(`"rule": "rm-rf", "match": "rm -rf x", "reason": "  "`), "exceptions[0].reason"},
		{"exception match that does not parse", exc(`"rule": "rm-rf", "match": "rm 'x", "reason": "x"`), "exceptions[0].match"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := guardrule.ParsePolicy([]byte(c.json))
			if err == nil {
				t.Fatal("no error")
			}
			if c.wantKey != "" && !strings.Contains(err.Error(), c.wantKey) {
				t.Errorf("error %q does not name %s", err, c.wantKey)
			}
		})
	}
}
