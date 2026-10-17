package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// contractVerdictError is a contract audit that reached no "conformant"
// verdict: not conformant (exit 2) or cannot decide (exit 1). The report
// is already printed; the error only carries the exit code and a line
// for stderr.
type contractVerdictError struct {
	verdict contract.Verdict
	reason  string
	failing int
}

func (e *contractVerdictError) Error() string {
	if e.verdict == contract.VerdictNotConformant {
		if e.failing == 1 {
			return "not conformant: 1 MUST rule fails"
		}
		return fmt.Sprintf("not conformant: %d MUST rules fail", e.failing)
	}
	return "cannot decide: " + e.reason
}

// verdictError returns the error vibe audit exits with for a report: nil
// when conformant.
func verdictError(rep *contract.Report) error {
	if rep.Verdict == contract.VerdictConformant {
		return nil
	}
	return &contractVerdictError{verdict: rep.Verdict, reason: rep.Reason, failing: rep.FailingMust}
}

// The --json report (spec 0052, "Report (#100)"). schema versions this
// shape, separately from the contract.
const contractJSONSchema = 1

type jsonContractReport struct {
	Schema          int                            `json:"schema"`
	Vibe            string                         `json:"vibe"`
	Contract        jsonContractVersions           `json:"contract"`
	StaticOnly      bool                           `json:"static_only"`
	Verdict         contract.Verdict               `json:"verdict"`
	Reason          string                         `json:"reason"`
	Summary         map[contract.Level]jsonSummary `json:"summary"`
	Rules           []jsonContractRule             `json:"rules"`
	GuardExceptions []jsonGuardException           `json:"guard_exceptions"`
	// GuardExceptionUses is new in spec 0053; a new key keeps schema 1.
	GuardExceptionUses []jsonGuardExceptionUse `json:"guard_exception_uses"`
	LocalAdjustments   []jsonLocalAdjustment   `json:"local_adjustments"`
}

type jsonContractVersions struct {
	Declared string `json:"declared"`
	Audited  string `json:"audited"`
}

// jsonSummary counts the rules of one level by result, every token
// present.
type jsonSummary map[contract.Result]int

type jsonContractRule struct {
	ID       string            `json:"id"`
	Level    contract.Level    `json:"level"`
	Category contract.Category `json:"category"`
	Title    string            `json:"title"`
	Result   contract.Result   `json:"result"`
	Parts    []jsonPart        `json:"parts"`
	Findings []jsonFinding     `json:"findings"`
	Waiver   *jsonWaiver       `json:"waiver"`
}

type jsonPart struct {
	Check  contract.PartKind `json:"check"`
	Result contract.Result   `json:"result"`
}

type jsonFinding struct {
	File    string `json:"file"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type jsonWaiver struct {
	Reason  string `json:"reason"`
	Until   string `json:"until"`
	Expired bool   `json:"expired"`
}

type jsonGuardException struct {
	Rule   string `json:"rule"`
	Match  string `json:"match"`
	Reason string `json:"reason"`
}

type jsonGuardExceptionUse struct {
	Rule  string `json:"rule"`
	Match string `json:"match"`
	Case  string `json:"case"`
}

type jsonLocalAdjustment struct {
	File    string `json:"file"`
	Kind    string `json:"kind"`
	Rule    string `json:"rule"`
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
}

func writeContractJSON(w io.Writer, rep *contract.Report, vibeVersion string) error {
	out := jsonContractReport{
		Schema:             contractJSONSchema,
		Vibe:               vibeVersion,
		Contract:           jsonContractVersions{Declared: rep.Contract.Declared, Audited: rep.Contract.Audited},
		StaticOnly:         rep.Static,
		Verdict:            rep.Verdict,
		Reason:             rep.Reason,
		Summary:            map[contract.Level]jsonSummary{},
		Rules:              []jsonContractRule{},
		GuardExceptions:    []jsonGuardException{},
		GuardExceptionUses: []jsonGuardExceptionUse{},
		LocalAdjustments:   []jsonLocalAdjustment{},
	}
	for _, level := range []contract.Level{contract.Must, contract.Should} {
		s := jsonSummary{}
		for _, result := range contract.Results {
			s[result] = rep.Count(level, result)
		}
		out.Summary[level] = s
	}
	for _, rr := range rep.Rules {
		jr := jsonContractRule{
			ID: rr.Rule.ID, Level: rr.Rule.Level, Category: rr.Rule.Category, Title: rr.Rule.Title,
			Result: rr.Result, Parts: []jsonPart{}, Findings: []jsonFinding{},
		}
		for _, p := range rr.Parts {
			jr.Parts = append(jr.Parts, jsonPart{Check: p.Kind, Result: p.Result})
		}
		for _, f := range rr.Findings() {
			jr.Findings = append(jr.Findings, jsonFinding(f))
		}
		if rr.Waiver != nil {
			jr.Waiver = &jsonWaiver{Reason: rr.Waiver.Reason, Until: rr.Waiver.Until.Format(time.DateOnly), Expired: rr.Waiver.Expired}
		}
		out.Rules = append(out.Rules, jr)
	}
	for _, g := range rep.GuardExceptions {
		out.GuardExceptions = append(out.GuardExceptions, jsonGuardException(g))
	}
	for _, u := range rep.GuardExceptionUses {
		out.GuardExceptionUses = append(out.GuardExceptionUses, jsonGuardExceptionUse(u))
	}
	for _, l := range rep.LocalAdjustments {
		out.LocalAdjustments = append(out.LocalAdjustments, jsonLocalAdjustment(l))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
