package contracttest

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// Audit loads the configuration at dir's vibe.yaml with contract.Load,
// through OpenRepo, runs contract.Audit with opts and returns the report.
// A configuration that does not load fails the test. Every report it
// returns has passed Check, so a rule's failing cases are checked for a
// file and a fix with no list to keep.
//
// opts is passed as is: nil Rules means the registry, and a nil
// Probe.Env means this process's environment. A test that runs
// execution or probe parts but must not depend on the programs this
// machine has passes Probe.Env: EmptyProbeEnv(t).
func Audit(t testing.TB, dir string, opts contract.Options) *contract.Report {
	t.Helper()
	ctx := context.Background()
	cfg, repo, err := contract.Load(ctx, dir, "", OpenRepo)
	if err != nil {
		t.Fatalf("loading the fixture's configuration: %v", err)
	}
	rep := contract.Audit(ctx, cfg, repo, opts)
	Check(t, rep)
	return rep
}

// Rules returns the registry's rules ids, in registry order, for
// Options.Rules: a test that runs probes waits only for the rules it
// reads. An ID not in the registry fails the test.
func Rules(t testing.TB, ids ...string) []contract.Rule {
	t.Helper()
	var rules []contract.Rule
	for _, r := range contract.Rules() {
		if slices.Contains(ids, r.ID) {
			rules = append(rules, r)
		}
	}
	if len(rules) != len(ids) {
		t.Fatalf("rules %v: some ID is not in the registry", ids)
	}
	return rules
}

// StandIn returns a copy of rules (nil for the registry) in which every
// part of the rules ids, execution and probe included, decides result
// with one finding that names a file and a fix. rules itself is left as
// it was. The execution and probe parts of a fixture find no program, so
// only stand-ins show the verdicts conformant and not conformant.
func StandIn(rules []contract.Rule, result contract.Result, ids ...string) []contract.Rule {
	if rules == nil {
		rules = contract.Rules()
	}
	out := slices.Clone(rules)
	for i := range out {
		if !slices.Contains(ids, out[i].ID) {
			continue
		}
		parts := slices.Clone(out[i].Parts)
		for j := range parts {
			parts[j].Run = func(*contract.Env, contract.Needed) contract.PartResult {
				return contract.PartResult{Result: result, Findings: []contract.Finding{
					{File: "stand-in.txt", Message: "stand-in " + string(result), Fix: "none"},
				}}
			}
		}
		out[i].Parts = parts
	}
	return out
}

// RuleIDs lists the IDs of the rules of a level ("" for every level)
// except those named, in registry order.
func RuleIDs(level contract.Level, except ...string) []string {
	var ids []string
	for _, r := range contract.Rules() {
		if (level == "" || r.Level == level) && !slices.Contains(except, r.ID) {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// Check is the report check (spec 0052, "Report (#100)": "A failure with
// no file named is a defect"): it fails the test with one error for each
// problem Problems finds in rep.
func Check(t testing.TB, rep *contract.Report) {
	t.Helper()
	for _, p := range Problems(rep) {
		t.Errorf("a failure must name a file and a fix: %s", p)
	}
}

// Problems lists the failed parts in a report that have no finding, and
// the findings of failed parts that name no file or no fix. It reads the
// report model, not the JSON, because only the model says which part a
// finding belongs to: a rule that fails statically may also carry an
// undecided part's finding, which names no file.
func Problems(rep *contract.Report) []string {
	var out []string
	for _, rr := range rep.Rules {
		for _, p := range rr.Parts {
			if p.Result != contract.ResultFail {
				continue
			}
			if len(p.Findings) == 0 {
				out = append(out, rr.Rule.ID+" "+string(p.Kind)+" fails with no finding")
			}
			for _, f := range p.Findings {
				if m := missing(f); m != "" {
					out = append(out, rr.Rule.ID+" finding "+f.Message+" has no "+m)
				}
			}
		}
	}
	return out
}

// missing names what a failed part's finding lacks: "file", "fix",
// "file and no fix", or "" for nothing.
func missing(f contract.Finding) string {
	var m []string
	if f.File == "" {
		m = append(m, "file")
	}
	if f.Fix == "" {
		m = append(m, "fix")
	}
	return strings.Join(m, " and no ")
}

// EmptyProbeEnv is this process's environment with PATH set to a new
// empty directory and CLAUDE_CODE_GIT_BASH_PATH removed, for
// ProbeOptions.Env: the runs of the execution and probe parts find no
// program, so a fixture's result does not depend on what this machine
// has installed.
func EmptyProbeEnv(t testing.TB) []string {
	t.Helper()
	return ProbeEnvWithPath(t.TempDir())
}

// ProbeEnvWithPath is this process's environment with PATH replaced and
// CLAUDE_CODE_GIT_BASH_PATH removed, for ProbeOptions.Env.
func ProbeEnvWithPath(path string) []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(name, "PATH") || strings.EqualFold(name, "CLAUDE_CODE_GIT_BASH_PATH") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "PATH="+path)
}
