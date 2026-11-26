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
// a deny case is MINOR. The cases marked with a probe belong to that
// rule alone (A-09 since contract 1.2.0, spec 0061): A-02, A-03, A-08
// and R-01 read only the unmarked cases, so a later MINOR adds deny
// cases of its own without changing them.

//go:embed guard_corpus.json
var guardCorpusJSON []byte

// What a corpus case expects of the guard.
const (
	WantDeny  = "deny"
	WantAllow = "allow"
)

// The unlock state a case runs in (A-08).
const (
	unlockNone    = "none"
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
	// Unlock is unlockNone, UnlockValid or UnlockExpired.
	Unlock string `json:"unlock,omitempty"`
	// Probe is the contract rule whose probe alone reads the case, one of
	// corpusProbes, or "" for the cases of A-02, A-03, A-08 and R-01.
	Probe string `json:"probe,omitempty"`
	// Fixed marks a deny case that no exception can lift: the self-unlock
	// commands and the edits of the unlock marker (spec 0072, "Guard
	// corpus").
	Fixed bool `json:"fixed,omitempty"`
	// Refuses are the simple commands of a deny case that its rule
	// refuses, each written as the simple command's words
	// (guardrule.Command.String). A command case that decomposes into more
	// than one simple command has them; any other case has none.
	Refuses []string `json:"refuses,omitempty"`
}

// ProbeA09 marks the cases of A-09: commands that skip Git hooks.
const ProbeA09 = "A-09"

// corpusProbes are the rules that own corpus cases of their own, and the
// guard rule ID their deny cases name.
var corpusProbes = map[string]string{ProbeA09: guardrule.GitSkipHooks}

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

// GuardCorpus returns every case of the guard corpus, in file order: the
// cases of A-02 and those marked with a probe.
func GuardCorpus() []GuardCase { return slices.Clone(guardCorpus()) }

// GuardCorpusFor returns the cases probe reads, in file order; "" gives
// the cases of A-02, A-03, A-08 and R-01.
func GuardCorpusFor(probe string) []GuardCase { return slices.Clone(corpusFor(probe)) }

func corpusFor(probe string) []GuardCase {
	var out []GuardCase
	for _, c := range guardCorpus() {
		if c.Probe == probe {
			out = append(out, c)
		}
	}
	return out
}

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
			c.Unlock = unlockNone
		}
		if err := checkGuardCase(*c, seen); err != nil {
			return nil, fmt.Errorf("case %d (%q): %w", i, c.Name, err)
		}
		seen[c.Name] = true
	}
	return doc.Cases, nil
}

func checkGuardCase(c GuardCase, seen map[string]bool) error { //nolint:gocyclo // baseline: pre-existing complexity
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
	case c.Unlock != unlockNone && c.Unlock != UnlockValid && c.Unlock != UnlockExpired:
		return fmt.Errorf("unlock is %q; it must be none, valid or expired", c.Unlock)
	case c.Probe != "" && corpusProbes[c.Probe] == "":
		return fmt.Errorf("probe is %q; it must be %s or absent", c.Probe, ProbeA09)
	}
	if _, err := guardrule.ParseTarget(tool, c.Payload.ToolInput); err != nil {
		return err
	}
	if c.Want == WantAllow {
		switch {
		case c.Rule != "":
			return fmt.Errorf("an allow case has rule %q", c.Rule)
		case c.Fixed:
			return errors.New("an allow case is fixed; only a deny case can be")
		case len(c.Refuses) > 0:
			return errors.New("an allow case has refuses; only a deny case can")
		}
		return nil
	}
	switch {
	case c.Rule == "":
		return errors.New("a deny case has no rule")
	case !guardrule.IsContractID(c.Rule):
		return fmt.Errorf("rule %q is not a guard rule ID", c.Rule)
	case c.Probe != "" && c.Rule != corpusProbes[c.Probe]:
		return fmt.Errorf("a deny case of %s has rule %q; it must be %s", c.Probe, c.Rule, corpusProbes[c.Probe])
	case c.Probe == "" && c.Rule == guardrule.GitSkipHooks:
		return fmt.Errorf("rule %s belongs to the cases of %s; mark the case with \"probe\": %q",
			c.Rule, ProbeA09, ProbeA09)
	case c.Rule == guardrule.GuardrailSettings:
		// A file edit of the settings, or a command that unlocks (A-08).
	case guardrule.IsPathRule(c.Rule) != guardrule.IsEditTool(tool):
		return fmt.Errorf("rule %s does not apply to %s calls", c.Rule, tool)
	}
	return checkRefuses(c)
}

// checkRefuses checks a deny case's refuses: present on a command case
// of more than one simple command, absent otherwise, and each one of the
// case's simple commands, once.
func checkRefuses(c GuardCase) error {
	t := c.Target()
	if !guardrule.IsCommandTool(t.Tool) {
		if len(c.Refuses) > 0 {
			return fmt.Errorf("a %s case has refuses; only a command case can", t.Tool)
		}
		return nil
	}
	cmds, err := guardrule.Commands(guardrule.ShellFor(t.Tool), t.Command)
	if err != nil {
		return err
	}
	simple := make([]string, len(cmds))
	for i, cmd := range cmds {
		simple[i] = cmd.String()
	}
	switch {
	case len(cmds) > 1 && len(c.Refuses) == 0:
		return fmt.Errorf("the command has %d simple commands %q; refuses must name those its rule refuses", len(cmds), simple)
	case len(cmds) <= 1 && len(c.Refuses) > 0:
		return errors.New("the command is one simple command; refuses is only for a compound command")
	}
	for i, r := range c.Refuses {
		switch {
		case !slices.Contains(simple, r):
			return fmt.Errorf("refuses %q is not one of the simple commands %q", r, simple)
		case slices.Contains(c.Refuses[:i], r):
			return fmt.Errorf("refuses names %q twice", r)
		}
	}
	return nil
}
