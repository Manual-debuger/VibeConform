package guardrule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
// rule off. NewRule and NewException build the rules and exceptions. The
// error is a *PolicyError.
func ParsePolicy(data []byte) (*Policy, error) {
	top, err := object(data)
	if err != nil {
		return nil, &PolicyError{Msg: "is not a JSON object: " + err.Error()}
	}
	if err := knownKeys("", top, "version", "mode", "rules", "exceptions"); err != nil {
		return nil, err
	}
	p := &Policy{Version: 2, Mode: Enforce}
	if err := checkVersion(top); err != nil {
		return nil, err
	}
	if raw, ok := top["mode"]; ok {
		var m string
		if json.Unmarshal(raw, &m) != nil || (Mode(m) != Enforce && Mode(m) != Observe) {
			return nil, policyErr("mode", "is %s; it must be \"enforce\" or \"observe\"", compact(raw))
		}
		p.Mode = Mode(m)
	}
	seen := map[string]bool{}
	p.Rules, err = parseList(top, "rules", func(at string, raw json.RawMessage) (Rule, error) {
		r, err := parseRule(at, raw)
		if err == nil && seen[r.ID()] {
			return Rule{}, policyErr(at+".id", "%q is used by an earlier rule", r.ID())
		}
		seen[r.ID()] = true
		return r, err
	})
	if err != nil {
		return nil, err
	}
	if p.Exceptions, err = parseList(top, "exceptions", parseException); err != nil {
		return nil, err
	}
	return p, nil
}

func checkVersion(top map[string]json.RawMessage) error {
	raw, ok := top["version"]
	if !ok {
		return policyErr("version", "is missing; it must be 2")
	}
	if v := strings.TrimSpace(string(raw)); v != "2" {
		return policyErr("version", "is %s; it must be 2", v)
	}
	return nil
}

// parseList reads the list at key with parse, which reads each entry at
// "key[i]". A missing key is an empty list.
func parseList[T any](top map[string]json.RawMessage, key string, parse func(at string, raw json.RawMessage) (T, error)) ([]T, error) {
	raw, ok := top[key]
	if !ok {
		return nil, nil
	}
	entries, err := list(key, raw)
	if err != nil {
		return nil, err
	}
	var out []T
	for i, e := range entries {
		v, err := parse(fmt.Sprintf("%s[%d]", key, i), e)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// fields reads the values of a rule or an exception. It keeps the first
// value whose JSON is not what the key needs, with its place in order,
// the order in which policy.json's keys are read. An invalid value that
// the constructor refuses at an earlier place comes first, so
// that a policy with several faults names the one it named before the
// constructors.
type fields struct {
	at    string
	m     map[string]json.RawMessage
	order []string
	err   error
	errAt int
}

func (f *fields) fail(key string, err error) {
	if f.err == nil {
		f.err, f.errAt = err, slices.Index(f.order, key)
	}
}

// str reads the string at key, which must be there and not be empty.
func (f *fields) str(key string) string {
	s, err := requiredString(f.at+"."+key, f.m[key])
	if err != nil {
		f.fail(key, err)
	}
	return s
}

// first returns the first fault: the JSON fault kept, or the
// constructor's refusal err if it comes before it.
func (f *fields) first(err error) error {
	var fe *FieldError
	if !errors.As(err, &fe) || f.err != nil && slices.Index(f.order, fe.Field) >= f.errAt {
		return firstErr(f.err, err)
	}
	key := f.at
	if fe.Field != "" {
		key += "." + fe.Field
	}
	return &PolicyError{Key: key, Msg: fe.Msg}
}

// firstErr is the first error that is not nil.
func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

// ruleOrder is the order in which parseRule reads a rule's keys; ""
// stands for the choice of command or path.
var ruleOrder = []string{"id", "level", "reason", "", "command", "path", "args", "tools"}

func parseRule(at string, raw json.RawMessage) (Rule, error) {
	m, err := object(raw)
	if err != nil {
		return Rule{}, policyErr(at, "is not an object")
	}
	if err := knownKeys(at+".", m, "id", "level", "tools", "reason", "command", "path", "args"); err != nil {
		return Rule{}, err
	}
	f := &fields{at: at, m: m, order: ruleOrder}
	s := RuleSpec{ID: f.str("id"), Level: Level(f.str("level")), Reason: f.str("reason")}
	_, hasCommand := m["command"]
	_, hasPath := m["path"]
	switch {
	case hasCommand && !hasPath:
		s.Kind, s.Pattern = CommandRule, f.str("command")
	case hasPath && !hasCommand:
		s.Kind, s.Pattern = PathRule, f.str("path")
	}
	if _, ok := m["args"]; ok && s.Kind == PathRule {
		f.fail("args", policyErr(at+".args", argsOnPathRule))
	} else if ok {
		s.Args = f.str("args")
	}
	if raw, ok := m["tools"]; ok {
		if json.Unmarshal(raw, &s.Tools) != nil || s.Tools == nil {
			f.fail("tools", policyErr(at+".tools", "is not a list of tool names"))
		}
	}
	r, err := NewRule(s)
	if err = f.first(err); err != nil {
		return Rule{}, err
	}
	return r, nil
}

var exceptionOrder = []string{"rule", "match", "reason"}

func parseException(at string, raw json.RawMessage) (Exception, error) {
	m, err := object(raw)
	if err != nil {
		return Exception{}, policyErr(at, "is not an object")
	}
	if err := knownKeys(at+".", m, "rule", "match", "reason"); err != nil {
		return Exception{}, err
	}
	f := &fields{at: at, m: m, order: exceptionOrder}
	e, err := NewException(f.str("rule"), f.str("match"), f.str("reason"))
	if err = f.first(err); err != nil {
		return Exception{}, err
	}
	return e, nil
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
