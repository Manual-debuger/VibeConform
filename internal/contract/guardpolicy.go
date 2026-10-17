package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The guard policy as A-04 and the report see it (spec 0052,
// "Recognition rules", Guard policy and Local guardrail adjustments).

const (
	guardPolicyFile = ".claude/hooks/policy.json"
	// localPolicyFile is the repository's own guard adjustments (spec
	// 0050). The report lists them; no rule judges them in Phase 1.
	localPolicyFile = ".claude/hooks/policy.local.json"
)

// guardPolicy is policy.json read as A-04 reads it: an object whose
// version and exceptions are the contract's, and whose other keys are
// the guard's.
type guardPolicy struct {
	top map[string]json.RawMessage
}

// readGuardPolicy reads policy.json. When it cannot, ok is false and
// result is A-04's: fail when it is not tracked or does not parse as a
// JSON object, cannot decide when it cannot be read.
func (e *Env) readGuardPolicy() (p *guardPolicy, result PartResult, ok bool) {
	data, res, ok := readDocument(e, guardPolicyFile, Finding{
		File:    guardPolicyFile,
		Message: "is not tracked, so the guard has no committed policy",
		Fix:     "create " + guardPolicyFile + " as the guard's version 2 policy, with any exceptions listed under exceptions",
	})
	if !ok {
		return nil, res, false
	}
	top, err := jsonObject(data)
	if err != nil {
		return nil, Fail(ParseFailure(guardPolicyFile, err)), false
	}
	return &guardPolicy{top: top}, Pass(), true
}

// jsonObject decodes data as a JSON object.
func jsonObject(data []byte) (map[string]json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, errors.New("the top level must be a JSON object")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	return top, nil
}

// exceptions returns the policy's exceptions list, which is optional. The
// error is set when exceptions is not a list.
func (p *guardPolicy) exceptions() ([]json.RawMessage, error) {
	raw, ok := p.top["exceptions"]
	if !ok {
		return nil, nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil || string(raw) == "null" {
		return nil, errors.New("exceptions is not a list")
	}
	return list, nil
}

// checkA04: policy.json is a JSON object with version 2. exceptions is
// optional and is a list; each entry names a guard rule ID and has a
// non-empty match and reason. Other keys are the guard's and not read.
func checkA04(env *Env) PartResult {
	p, res, ok := env.readGuardPolicy()
	if !ok {
		return res
	}
	var findings []Finding
	fail := func(msg, fix string) {
		findings = append(findings, Finding{File: guardPolicyFile, Message: msg, Fix: fix})
	}
	const versionFix = "make the guard policy version 2, with every exception listed under exceptions " +
		"(policy.local.json keeps version 1)"
	switch raw, ok := p.top["version"]; {
	case !ok:
		fail("has no version; contract "+Implemented+" needs version 2", versionFix)
	case strings.TrimSpace(string(raw)) != "2":
		fail(fmt.Sprintf("has version %s; contract %s needs version 2", strings.TrimSpace(string(raw)), Implemented), versionFix)
	}

	list, err := p.exceptions()
	if err != nil {
		fail(err.Error(), "make exceptions a list of objects with rule, match and reason")
	}
	for i, raw := range list {
		at := fmt.Sprintf("exceptions[%d]", i)
		var ex map[string]any
		if err := json.Unmarshal(raw, &ex); err != nil || ex == nil {
			fail(at+" is not an object", "write each exception as an object with rule, match and reason")
			continue
		}
		rule, _ := ex["rule"].(string)
		if !guardrule.IsContractID(rule) {
			fail(fmt.Sprintf("%s.rule %s is not a guard rule ID", at, jsonText(ex["rule"])),
				"name one of the guard rule IDs: "+strings.Join(guardrule.IDs, ", "))
		}
		if m, _ := ex["match"].(string); m == "" {
			fail(at+".match is empty", "set match to what the exception lets through")
		}
		if r, _ := ex["reason"].(string); strings.TrimSpace(r) == "" {
			fail(at+".reason is empty", "say in reason why the guard is loosened")
		}
	}
	if len(findings) > 0 {
		return Fail(findings...)
	}
	return Pass()
}

// jsonText writes a decoded JSON value back as JSON, for messages.
func jsonText(v any) string {
	if v == nil {
		return "(missing)"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// guardListings returns what the report lists for review: the guard
// policy's exceptions (A-04) and the exceptions and extra rules of a
// tracked .claude/hooks/policy.local.json (spec 0050). It reads, and
// decides nothing: every exception is listed, valid or not.
func guardListings(env *Env) ([]GuardException, []LocalAdjustment) {
	return guardExceptions(env), localAdjustments(env)
}

func guardExceptions(env *Env) []GuardException {
	data, err := env.ReadFile(guardPolicyFile)
	if err != nil {
		return nil
	}
	top, err := jsonObject(data)
	if err != nil {
		return nil
	}
	list, err := (&guardPolicy{top: top}).exceptions()
	if err != nil {
		return nil
	}
	var out []GuardException
	for _, raw := range list {
		var ex map[string]any
		if json.Unmarshal(raw, &ex) != nil || ex == nil {
			continue
		}
		out = append(out, GuardException{Rule: stringField(ex, "rule"), Match: stringField(ex, "match"), Reason: stringField(ex, "reason")})
	}
	return out
}

// localAdjustments lists a tracked policy.local.json's exceptions, then
// its extra rules, in file order. Its version is not judged. A file that
// does not parse lists nothing and adds a note to the report.
func localAdjustments(env *Env) []LocalAdjustment {
	data, err := env.ReadFile(localPolicyFile)
	if err != nil {
		return nil
	}
	var local struct {
		Rules      []map[string]any `json:"rules"`
		Exceptions []map[string]any `json:"exceptions"`
	}
	_, err = jsonObject(data)
	if err == nil {
		err = json.Unmarshal(data, &local)
	}
	if err != nil {
		env.Note("%s does not parse (%v), so its guardrail adjustments are not listed", localPolicyFile, err)
		return nil
	}
	var out []LocalAdjustment
	for _, ex := range local.Exceptions {
		out = append(out, LocalAdjustment{File: localPolicyFile, Kind: "exception",
			Rule: stringField(ex, "rule"), Pattern: stringField(ex, "pattern"), Reason: stringField(ex, "reason")})
	}
	for _, r := range local.Rules {
		out = append(out, LocalAdjustment{File: localPolicyFile, Kind: "rule",
			Rule: stringField(r, "id"), Pattern: stringField(r, "pattern")})
	}
	return out
}

// stringField returns a string field of a decoded JSON object, or "".
func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
