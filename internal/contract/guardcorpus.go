package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The contract's guard corpus (spec 0051, "The guard corpus"; spec 0053,
// "The contract's guard corpus"). It belongs to the contract, not to a
// guard: A-02 and A-03 feed it to whatever PreToolUse hooks a repository
// declares. Adding a deny case is MAJOR; adding an allow case or removing
// a deny case is MINOR.

//go:embed guard_corpus.json
var guardCorpusJSON []byte

// What a corpus case expects of the guard.
const (
	WantDeny  = "deny"
	WantAllow = "allow"
)

// The unlock state a case runs in (A-08).
const (
	UnlockNone    = "none"
	UnlockValid   = "valid"
	UnlockExpired = "expired"
)

// GuardCase is one case of the guard corpus.
type GuardCase struct {
	Name string `json:"name"`
	// Want is WantDeny or WantAllow.
	Want    string       `json:"want"`
	Payload GuardPayload `json:"payload"`
	// Rule is the guard rule ID of a deny case, and empty for an allow
	// case.
	Rule string `json:"rule,omitempty"`
	// Unlock is UnlockNone, UnlockValid or UnlockExpired.
	Unlock string `json:"unlock,omitempty"`
}

// GuardPayload is the Claude Code tool call of a case. The audit adds
// the other fields of a PreToolUse payload.
type GuardPayload struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// Target is what the case's tool call acts on, for exception matching.
func (c GuardCase) Target() guardrule.Target {
	t, _ := guardrule.ParseTarget(c.Payload.ToolName, c.Payload.ToolInput)
	return t
}

var guardCorpus = sync.OnceValue(func() []GuardCase {
	cases, err := parseGuardCorpus(guardCorpusJSON)
	if err != nil {
		panic("the embedded guard corpus is invalid: " + err.Error())
	}
	return cases
})

// GuardCorpus returns the guard corpus of contract 1.0.0, in file order.
func GuardCorpus() []GuardCase { return slices.Clone(guardCorpus()) }

// parseGuardCorpus reads and checks a corpus document.
func parseGuardCorpus(data []byte) ([]GuardCase, error) {
	var doc struct {
		Cases []GuardCase `json:"cases"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if len(doc.Cases) == 0 {
		return nil, errors.New("the corpus has no cases")
	}
	seen := map[string]bool{}
	for i := range doc.Cases {
		c := &doc.Cases[i]
		if c.Unlock == "" {
			c.Unlock = UnlockNone
		}
		if err := checkGuardCase(*c, seen); err != nil {
			return nil, fmt.Errorf("case %d (%q): %w", i, c.Name, err)
		}
		seen[c.Name] = true
	}
	return doc.Cases, nil
}

func checkGuardCase(c GuardCase, seen map[string]bool) error {
	tool := c.Payload.ToolName
	switch {
	case c.Name == "":
		return errors.New("has no name")
	case seen[c.Name]:
		return fmt.Errorf("the name %q is used by an earlier case", c.Name)
	case c.Want != WantDeny && c.Want != WantAllow:
		return fmt.Errorf("want is %q; it must be deny or allow", c.Want)
	case !slices.Contains(guardrule.A01Tools, tool):
		return fmt.Errorf("tool %q is not one of the A-01 tools", tool)
	case c.Unlock != UnlockNone && c.Unlock != UnlockValid && c.Unlock != UnlockExpired:
		return fmt.Errorf("unlock is %q; it must be none, valid or expired", c.Unlock)
	}
	if _, err := guardrule.ParseTarget(tool, c.Payload.ToolInput); err != nil {
		return err
	}
	if c.Want == WantAllow {
		if c.Rule != "" {
			return fmt.Errorf("an allow case has rule %q", c.Rule)
		}
		return nil
	}
	switch {
	case c.Rule == "":
		return errors.New("a deny case has no rule")
	case !guardrule.IsContractID(c.Rule):
		return fmt.Errorf("rule %q is not a guard rule ID", c.Rule)
	case c.Rule == guardrule.GuardrailSettings:
		// A file edit of the settings, or a command that unlocks (A-08).
	case guardrule.IsPathRule(c.Rule) != guardrule.IsEditTool(tool):
		return fmt.Errorf("rule %s does not apply to %s calls", c.Rule, tool)
	}
	return nil
}
