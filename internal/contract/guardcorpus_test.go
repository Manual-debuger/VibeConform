package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The embedded guard corpus is valid (spec 0053, "The contract's guard
// corpus"): names are unique, every case uses an A-01 tool and has a
// target, every deny case names one of the ten guard rule IDs of its
// tool's kind, and allow cases name none.
func TestGuardCorpusIsValid(t *testing.T) {
	cases := GuardCorpus()
	if len(cases) == 0 {
		t.Fatal("the corpus is empty")
	}
	for _, c := range cases {
		if !slices.Contains(guardrule.A01Tools, c.Payload.ToolName) {
			t.Errorf("%s: tool %s is not an A-01 tool", c.Name, c.Payload.ToolName)
		}
		if c.Want == WantDeny && !guardrule.IsContractID(c.Rule) {
			t.Errorf("%s: deny case has rule %q", c.Name, c.Rule)
		}
		if c.Unlock != UnlockNone && c.Unlock != UnlockValid && c.Unlock != UnlockExpired {
			t.Errorf("%s: unlock %q", c.Name, c.Unlock)
		}
		// Every command parses, so that a shell-parsing guard can decide
		// it by its rules rather than refuse it as a parse error.
		if tgt := c.Target(); guardrule.IsCommandTool(tgt.Tool) {
			if _, err := guardrule.Commands(guardrule.ShellFor(tgt.Tool), tgt.Command); err != nil {
				t.Errorf("%s: %v", c.Name, err)
			}
		}
	}
}

// The counts spec 0053 records for the corpus are the corpus's counts.
func TestGuardCorpusCountsAreInSpec0053(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "0053-contract-probes-and-guard.md"))
	if err != nil {
		t.Fatal(err)
	}
	section := string(data)
	start := strings.Index(section, "### The contract's guard corpus")
	if start < 0 {
		t.Fatal("spec 0053 has no corpus section")
	}
	section = section[start:]
	if end := strings.Index(section[4:], "\n### "); end >= 0 {
		section = section[:end+4]
	}

	deny, allow := 0, 0
	byRule := map[string]int{}
	for _, c := range GuardCorpus() {
		if c.Want == WantDeny {
			deny++
			byRule[c.Rule]++
		} else {
			allow++
		}
	}
	want := fmt.Sprintf("%d cases: %d deny and %d allow", deny+allow, deny, allow)
	if !strings.Contains(strings.Join(strings.Fields(section), " "), want) {
		t.Errorf("spec 0053's corpus section does not say %q", want)
	}
	row := regexp.MustCompile("(?m)^[ \\t]*\\| `([a-z-]+)` \\| (\\d+) \\|\\r?$")
	got := map[string]int{}
	for _, m := range row.FindAllStringSubmatch(section, -1) {
		n, _ := strconv.Atoi(m[2])
		got[m[1]] = n
	}
	for _, id := range guardrule.IDs {
		if got[id] != byRule[id] {
			t.Errorf("spec 0053 records %d deny cases for %s; the corpus has %d", got[id], id, byRule[id])
		}
	}
}

func TestParseGuardCorpusRejects(t *testing.T) {
	bash := func(cmd string) string {
		return `{"tool_name": "Bash", "tool_input": {"command": "` + cmd + `"}}`
	}
	write := `{"tool_name": "Write", "tool_input": {"file_path": ".env", "content": ""}}`
	cases := []struct {
		name, json, want string
	}{
		{"not JSON", `{`, ""},
		{"no cases", `{"cases": []}`, "no cases"},
		{"unknown key", `{"cases": [{"name": "a", "want": "allow", "payload": ` + bash("ls") + `, "rul": "x"}]}`, "rul"},
		{"no name", `{"cases": [{"want": "allow", "payload": ` + bash("ls") + `}]}`, "name"},
		{"duplicate name", `{"cases": [{"name": "a", "want": "allow", "payload": ` + bash("ls") + `},
			{"name": "a", "want": "allow", "payload": ` + bash("pwd") + `}]}`, `"a"`},
		{"unknown want", `{"cases": [{"name": "a", "want": "ask", "payload": ` + bash("ls") + `}]}`, "want"},
		{"deny without rule", `{"cases": [{"name": "a", "want": "deny", "payload": ` + bash("rm -rf x") + `}]}`, "rule"},
		{"deny with an unknown rule", `{"cases": [{"name": "a", "want": "deny", "rule": "rm-r", "payload": ` + bash("rm -rf x") + `}]}`, "rm-r"},
		{"deny with an internal rule", `{"cases": [{"name": "a", "want": "deny", "rule": "parse-error", "payload": ` + bash("rm '") + `}]}`, "parse-error"},
		{"allow with a rule", `{"cases": [{"name": "a", "want": "allow", "rule": "rm-rf", "payload": ` + bash("ls") + `}]}`, "rule"},
		{"tool not in A-01", `{"cases": [{"name": "a", "want": "allow", "payload": {"tool_name": "NotebookEdit", "tool_input": {"notebook_path": "n"}}}]}`, "NotebookEdit"},
		{"payload without its input", `{"cases": [{"name": "a", "want": "allow", "payload": {"tool_name": "Bash", "tool_input": {"cmd": "ls"}}}]}`, "command"},
		{"path rule on a command", `{"cases": [{"name": "a", "want": "deny", "rule": "secret-file", "payload": ` + bash("cat .env") + `}]}`, "secret-file"},
		{"command rule on a file edit", `{"cases": [{"name": "a", "want": "deny", "rule": "rm-rf", "payload": ` + write + `}]}`, "rm-rf"},
		{"unknown unlock", `{"cases": [{"name": "a", "want": "allow", "unlock": "open", "payload": ` + bash("ls") + `}]}`, "unlock"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseGuardCorpus([]byte(c.json))
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %s", err, c.want)
			}
		})
	}
}

func TestParseGuardCorpusDefaultsUnlock(t *testing.T) {
	cases, err := parseGuardCorpus([]byte(`{"cases": [
		{"name": "a", "want": "deny", "rule": "guardrail-settings", "unlock": "expired",
		 "payload": {"tool_name": "Write", "tool_input": {"file_path": ".claude/settings.json", "content": "{}"}}},
		{"name": "b", "want": "allow", "payload": {"tool_name": "Bash", "tool_input": {"command": "ls"}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if cases[0].Unlock != UnlockExpired || cases[1].Unlock != UnlockNone {
		t.Errorf("unlocks = %q, %q", cases[0].Unlock, cases[1].Unlock)
	}
	if got := cases[0].Target(); got != (guardrule.Target{Tool: "Write", Path: ".claude/settings.json"}) {
		t.Errorf("target = %+v", got)
	}
}
