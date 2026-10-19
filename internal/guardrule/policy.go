package guardrule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Mode is policy.json's mode.
type Mode string

const (
	// Enforce refuses what a rule refuses. It is the default.
	Enforce Mode = "enforce"
	// Observe only reports what would be refused.
	Observe Mode = "observe"
)

// Level is what a repository rule does when it matches.
type Level string

const (
	// Deny refuses the call.
	Deny Level = "deny"
	// Ask lets Claude Code ask the person.
	Ask Level = "ask"
)

// Policy is .claude/hooks/policy.json version 2 as vibe-guard reads it
// (spec 0053, "policy.json version 2").
type Policy struct {
	Version    int
	Mode       Mode
	Rules      []Rule
	Exceptions []Exception
}

// Rule is one of the repository's own rules. Exactly one of Command and
// Path is set. Tools is the list given, or nil for every tool of the
// rule's kind.
type Rule struct {
	ID     string
	Level  Level
	Tools  []string
	Reason string
	// Command is a word prefix, matched as exceptions match it.
	Command string
	// Args is a regular expression that some argument after the prefix
	// must match. Only with Command.
	Args string
	// Path is a path glob (*, ?, **), matched against the repository
	// relative path.
	Path string

	args *regexp.Regexp
}

// Exception lifts one contract rule where Match matches (spec 0051,
// "Guard policy and exceptions"; spec 0053 decides what match means).
type Exception struct {
	Rule   string
	Match  string
	Reason string
}

// PolicyError is a policy.json that vibe-guard cannot use. Key names the
// offending key ("rules[1].level"), or is empty for the document itself.
type PolicyError struct {
	Key string
	Msg string
}

func (e *PolicyError) Error() string {
	if e.Key == "" {
		return "policy.json " + e.Msg
	}
	return "policy.json: " + e.Key + " " + e.Msg
}

func policyErr(key, format string, args ...any) error {
	return &PolicyError{Key: key, Msg: fmt.Sprintf(format, args...)}
}

// ParsePolicy reads policy.json version 2. Unknown keys and invalid
// values are errors that name the key, so a typo cannot silently turn a
// rule off. The error is a *PolicyError.
func ParsePolicy(data []byte) (*Policy, error) {
	top, err := object(data)
	if err != nil {
		return nil, &PolicyError{Msg: "is not a JSON object: " + err.Error()}
	}
	if err := knownKeys("", top, "version", "mode", "rules", "exceptions"); err != nil {
		return nil, err
	}
	p := &Policy{Mode: Enforce}
	raw, ok := top["version"]
	if !ok {
		return nil, policyErr("version", "is missing; it must be 2")
	}
	if strings.TrimSpace(string(raw)) != "2" {
		return nil, policyErr("version", "is %s; it must be 2", strings.TrimSpace(string(raw)))
	}
	p.Version = 2
	if raw, ok := top["mode"]; ok {
		var m string
		if json.Unmarshal(raw, &m) != nil || (Mode(m) != Enforce && Mode(m) != Observe) {
			return nil, policyErr("mode", "is %s; it must be \"enforce\" or \"observe\"", compact(raw))
		}
		p.Mode = Mode(m)
	}
	if raw, ok := top["rules"]; ok {
		list, err := list("rules", raw)
		if err != nil {
			return nil, err
		}
		for i, r := range list {
			rule, err := parseRule(fmt.Sprintf("rules[%d]", i), r)
			if err != nil {
				return nil, err
			}
			if slices.ContainsFunc(p.Rules, func(o Rule) bool { return o.ID == rule.ID }) {
				return nil, policyErr(fmt.Sprintf("rules[%d].id", i), "%q is used by an earlier rule", rule.ID)
			}
			p.Rules = append(p.Rules, rule)
		}
	}
	if raw, ok := top["exceptions"]; ok {
		list, err := list("exceptions", raw)
		if err != nil {
			return nil, err
		}
		for i, e := range list {
			ex, err := parseException(fmt.Sprintf("exceptions[%d]", i), e)
			if err != nil {
				return nil, err
			}
			p.Exceptions = append(p.Exceptions, ex)
		}
	}
	return p, nil
}

func parseRule(at string, raw json.RawMessage) (Rule, error) {
	m, err := object(raw)
	if err != nil {
		return Rule{}, policyErr(at, "is not an object")
	}
	if err := knownKeys(at+".", m, "id", "level", "tools", "reason", "command", "path", "args"); err != nil {
		return Rule{}, err
	}
	var r Rule
	if r.ID, err = requiredString(at+".id", m["id"]); err != nil {
		return Rule{}, err
	}
	if IsContractID(r.ID) || IsInternalID(r.ID) {
		return Rule{}, policyErr(at+".id", "%q is a built-in guard rule ID; a repository rule needs its own", r.ID)
	}
	level, err := requiredString(at+".level", m["level"])
	if err != nil {
		return Rule{}, err
	}
	if r.Level = Level(level); r.Level != Deny && r.Level != Ask {
		return Rule{}, policyErr(at+".level", "is %q; it must be \"deny\" or \"ask\"", level)
	}
	if r.Reason, err = requiredString(at+".reason", m["reason"]); err != nil {
		return Rule{}, err
	}
	_, hasCommand := m["command"]
	_, hasPath := m["path"]
	switch {
	case hasCommand == hasPath:
		return Rule{}, policyErr(at, "must have exactly one of command and path")
	case hasCommand:
		if r.Command, err = requiredString(at+".command", m["command"]); err != nil {
			return Rule{}, err
		}
		if err := checkMatch(r.Command); err != nil {
			return Rule{}, policyErr(at+".command", "does not parse as a command: %v", err)
		}
	default:
		if r.Path, err = requiredString(at+".path", m["path"]); err != nil {
			return Rule{}, err
		}
	}
	if raw, ok := m["args"]; ok {
		if hasPath {
			return Rule{}, policyErr(at+".args", "is only allowed with command")
		}
		if r.Args, err = requiredString(at+".args", raw); err != nil {
			return Rule{}, err
		}
		if r.args, err = regexp.Compile(r.Args); err != nil {
			return Rule{}, policyErr(at+".args", "is not a regular expression: %v", err)
		}
	}
	if raw, ok := m["tools"]; ok {
		var tools []string
		if json.Unmarshal(raw, &tools) != nil || tools == nil {
			return Rule{}, policyErr(at+".tools", "is not a list of tool names")
		}
		for _, tool := range tools {
			switch {
			case !IsCommandTool(tool) && !IsEditTool(tool):
				return Rule{}, policyErr(at+".tools", "names %q, which is not one of %s",
					tool, strings.Join(append(slices.Clone(CommandTools), EditTools...), ", "))
			case hasCommand && !IsCommandTool(tool):
				return Rule{}, policyErr(at+".tools", "names %q, which runs no command, on a command rule", tool)
			case hasPath && !IsEditTool(tool):
				return Rule{}, policyErr(at+".tools", "names %q, which edits no file, on a path rule", tool)
			}
		}
		r.Tools = tools
	}
	return r, nil
}

func parseException(at string, raw json.RawMessage) (Exception, error) {
	m, err := object(raw)
	if err != nil {
		return Exception{}, policyErr(at, "is not an object")
	}
	if err := knownKeys(at+".", m, "rule", "match", "reason"); err != nil {
		return Exception{}, err
	}
	var e Exception
	if e.Rule, err = requiredString(at+".rule", m["rule"]); err != nil {
		return Exception{}, err
	}
	if !IsContractID(e.Rule) {
		return Exception{}, policyErr(at+".rule", "%q is not a guard rule ID; it must be one of %s",
			e.Rule, strings.Join(IDs, ", "))
	}
	if e.Match, err = requiredString(at+".match", m["match"]); err != nil {
		return Exception{}, err
	}
	if !IsPathRule(e.Rule) {
		if err := checkMatch(e.Match); err != nil {
			return Exception{}, policyErr(at+".match", "does not parse as a command: %v", err)
		}
	}
	if e.Reason, err = requiredString(at+".reason", m["reason"]); err != nil {
		return Exception{}, err
	}
	return e, nil
}

// AppliesTo reports whether the rule reads calls of tool.
func (r Rule) AppliesTo(tool string) bool {
	if r.Tools != nil {
		return slices.Contains(r.Tools, tool)
	}
	if r.Command != "" {
		return IsCommandTool(tool)
	}
	return IsEditTool(tool)
}

// object decodes data as a JSON object.
func object(data []byte) (map[string]json.RawMessage, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return nil, errors.New("the top level is not an object")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func knownKeys(prefix string, m map[string]json.RawMessage, keys ...string) error {
	var unknown []string
	for k := range m {
		if !slices.Contains(keys, k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return policyErr(prefix+unknown[0], "is not a known key; the keys are %s", strings.Join(keys, ", "))
}

func list(key string, raw json.RawMessage) ([]json.RawMessage, error) {
	var l []json.RawMessage
	if json.Unmarshal(raw, &l) != nil || l == nil {
		return nil, policyErr(key, "is not a list")
	}
	return l, nil
}

func requiredString(key string, raw json.RawMessage) (string, error) {
	if raw == nil {
		return "", policyErr(key, "is missing")
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", policyErr(key, "is %s; it must be a string", compact(raw))
	}
	if strings.TrimSpace(s) == "" {
		return "", policyErr(key, "is empty")
	}
	return s, nil
}

func compact(raw json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return string(raw)
	}
	return b.String()
}
