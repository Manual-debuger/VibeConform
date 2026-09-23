package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Field names the part of a tool call a rule inspects. The guard maps a
// tool call's tool_name to one of these through Policy.Tools and matches
// the rule's Pattern against tool_input[field].
type Field string

const (
	// FieldCommand is the shell command of a Bash or PowerShell call.
	FieldCommand Field = "command"
	// FieldFilePath is the target file of a Write or Edit call.
	FieldFilePath Field = "file_path"
)

// Rule is one thing the guard denies.
//
// Pattern is matched against the parsed field. Raw is matched against the
// whole stdin payload when it cannot be parsed, or names a tool or field
// the policy doesn't know. Raw is the pattern the bash hooks used before
// spec 0021, which is what keeps the fallback exactly as strict as they
// were. For every command rule the two are the same; the file rule's Raw
// matches the JSON text around the path, because that is all the old hook
// could see.
type Rule struct {
	ID      string `json:"id"`
	Field   Field  `json:"field"`
	Pattern string `json:"pattern"`
	Raw     string `json:"raw"`
	Message string `json:"message"`
}

// Policy is the whole guard configuration, serialized into
// .claude/hooks/policy.json for the per-runtime guards to interpret. See
// docs/specs/0021-agent-hooks-task-interface.md.
type Policy struct {
	// Version is the policy.json layout, bumped only if an interpreter would
	// misread the new shape.
	Version int `json:"version"`
	// Tools maps each field to the tool_name values whose calls carry it.
	// MultiEdit is listed because Claude Code's matchers are regular
	// expressions: "Write|Edit" also routes MultiEdit calls to the guard,
	// and the bash hook they replace checked those too.
	Tools map[Field][]string `json:"tools"`
	Rules []Rule             `json:"rules"`
}

// commandRule builds a command rule, whose parsed and raw patterns are
// the same.
func commandRule(id, pattern, message string) Rule {
	return Rule{ID: id, Field: FieldCommand, Pattern: pattern, Raw: pattern, Message: message}
}

// policy is the single source of the guard's rules: a port, rule for rule,
// of block-dangerous.sh and block-secret-files.sh as they stood before spec
// 0021. POSIX classes ([[:space:]]) became \s, the only change, because
// Python and JavaScript regular expressions have no POSIX classes.
//
// Changing what is blocked is a policy decision, not a refactor: keep it
// out of commits that only move code (spec 0021, non-goals).
var policy = Policy{
	Version: 1,
	Tools: map[Field][]string{
		FieldCommand:  {"Bash", "PowerShell"},
		FieldFilePath: {"Edit", "MultiEdit", "Write"},
	},
	Rules: []Rule{
		commandRule("rm-rf",
			`rm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)(\s|"|$)`,
			"Blocked: rm -rf (or equivalent) is a destructive operation. Ask the user to run it themselves if intended."),
		commandRule("remove-item-recurse-force",
			`Remove-Item[^"\\]*-Recurse[^"\\]*-Force|Remove-Item[^"\\]*-Force[^"\\]*-Recurse`,
			"Blocked: Remove-Item -Recurse -Force is a destructive operation. Ask the user to run it themselves if intended."),
		commandRule("git-reset-hard",
			`git\s+reset\s+--hard`,
			"Blocked: git reset --hard discards uncommitted work. Ask the user to run it themselves if intended."),
		commandRule("git-clean-fd",
			`git\s+clean\s+-[a-zA-Z]*f[a-zA-Z]*d|git\s+clean\s+-[a-zA-Z]*d[a-zA-Z]*f`,
			"Blocked: git clean -fd (or equivalent) permanently deletes untracked files. Ask the user to run it themselves if intended."),
		commandRule("git-push-force",
			`git\s+push[^"\\]*(--force(\s|"|$)|\s-f(\s|"|$))`,
			"Blocked: git push --force can overwrite remote history. Ask the user to run it themselves if intended."),
		commandRule("git-branch-force-delete",
			`git\s+branch\s+-D`,
			"Blocked: git branch -D force-deletes a branch. Ask the user to run it themselves if intended."),
		{
			ID:      "secret-file",
			Field:   FieldFilePath,
			Pattern: `(\.env(\.[a-zA-Z]+)?|\.pem|\.key|id_rsa|id_ed25519|credentials\.json|\.npmrc|\.netrc)$`,
			Raw:     `"file_path"\s*:\s*"[^"]*(\.env(\.[a-zA-Z]+)?|\.pem|\.key|id_rsa|id_ed25519|credentials\.json|\.npmrc|\.netrc)"`,
			Message: "Blocked: this looks like a secret/credential file. Ask the user to edit it themselves.",
		},
	},
}

// renderPolicy serializes policy as .claude/hooks/policy.json content:
// two-space indent, LF, trailing newline, and no HTML escaping, so the
// patterns read in the file exactly as they are written above. Output is
// deterministic because the only map is keyed by Field, which encoding/json
// sorts.
func renderPolicy(p Policy) ([]byte, error) {
	for _, tools := range p.Tools {
		if !slices.IsSorted(tools) {
			return nil, fmt.Errorf("render policy: tool list %v is not sorted", tools)
		}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return nil, fmt.Errorf("render policy: %w", err)
	}
	return collapseStringArrays(buf.Bytes()), nil
}

// multilineStringArray matches an indented JSON array whose elements are
// all plain strings with no escapes, one per line, as json.Encoder writes
// them.
var multilineStringArray = regexp.MustCompile(`(?m)^( *)("[^"\\\n]*": )\[\n((?: *"[^"\\\n]*",?\n)+) *\]`)

// printWidth is the widest line collapseStringArrays will produce:
// Prettier's default print width, below prod-ts's configured 100, so the
// result is stable under either.
const printWidth = 80

// collapseStringArrays puts each short array of plain strings on one line,
// which is how Prettier formats JSON. prod-ts's fmt:check runs Prettier
// over **/*.json, .claude/ included, so a policy.json in json.Encoder's
// one-element-per-line style would fail every adopter's format check.
// Arrays that would not fit stay expanded, as Prettier leaves them.
func collapseStringArrays(b []byte) []byte {
	return multilineStringArray.ReplaceAllFunc(b, func(m []byte) []byte {
		parts := multilineStringArray.FindSubmatch(m)
		indent, key, body := parts[1], parts[2], parts[3]

		var elems []string
		for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
			elems = append(elems, strings.TrimSuffix(strings.TrimSpace(line), ","))
		}
		oneLine := string(indent) + string(key) + "[" + strings.Join(elems, ", ") + "]"
		if len(oneLine) > printWidth {
			return m
		}
		return []byte(oneLine)
	})
}
