package guardrule_test

import (
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

func TestContractIDs(t *testing.T) {
	want := []string{
		"rm-rf", "remove-item-recurse-force", "git-reset-hard", "git-clean-fd", "git-push-force",
		"git-branch-force-delete", "git-discard-worktree", "secret-file", "guard-policy-file", "guardrail-settings",
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
	if r := p.Rules[1]; r.ID != "ask-kubectl" || r.Level != guardrule.Ask || r.Command != "kubectl" || r.Args != "^--context=prod" {
		t.Errorf("rule = %+v", r)
	}
	if e := p.Exceptions[0]; e.Rule != "git-clean-fd" || e.Match != "git clean -fdx build/" || e.Reason != "build output" {
		t.Errorf("exception = %+v", e)
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
