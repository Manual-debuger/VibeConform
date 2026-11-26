package contract_test

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The runner runs the execution and probe parts in rule-list order, and
// the one last part after all of them (spec 0071). These tests run
// contract.Audit over stand-in rules in Options.Rules and read the order
// in which the stand-ins were called and what the last part saw.

// runnerFixture is a Git repository holding only a go.mod and a
// vibe.yaml with no agents, so A-02 is not applicable; and its parsed
// configuration.
func runnerFixture(t *testing.T) (*contract.Config, *contract.Repo) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := contracttest.GitFixture(t, t.TempDir(),
		contracttest.WithFile("vibe.yaml", "contract: 1.2.0\nplatforms: [linux]\nagents: []\n"),
		contracttest.WithFile("go.mod", "module fixture\n\ngo 1.22\n"))
	cfg, repo, err := contract.Load(context.Background(), dir, "", contracttest.OpenRepo)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, repo
}

// partName names a part as the runner's messages do: "<id>/<kind>".
func partName(id string, kind contract.PartKind) string {
	return id + "/" + string(kind)
}

// runner records the order in which stand-in parts are called.
type runner struct {
	called []string
}

// part is a stand-in part with Run that records its call and fails with
// "from <id>".
func (r *runner) part(id string, kind contract.PartKind) contract.Part {
	return contract.Part{Kind: kind, Run: func(*contract.Env) contract.PartResult {
		r.called = append(r.called, partName(id, kind))
		return contract.Fail(contract.Finding{Message: "from " + id})
	}}
}

// last is a stand-in last part that records its call and runs check on
// the earlier results.
func (r *runner) last(id string, kind contract.PartKind, check func(contract.Earlier) contract.PartResult) contract.Part {
	return contract.Part{Kind: kind, RunLast: func(_ *contract.Env, earlier contract.Earlier) contract.PartResult {
		r.called = append(r.called, partName(id, kind))
		return check(earlier)
	}}
}

// rule is a stand-in rule of one part with Run.
func (r *runner) rule(id string, kind contract.PartKind) contract.Rule {
	return contract.Rule{ID: id, Parts: []contract.Part{r.part(id, kind)}}
}

// lastRule is a stand-in rule of one last part.
func (r *runner) lastRule(id string, kind contract.PartKind, check func(contract.Earlier) contract.PartResult) contract.Rule {
	return contract.Rule{ID: id, Parts: []contract.Part{r.last(id, kind, check)}}
}

func runAudit(t *testing.T, cfg *contract.Config, repo *contract.Repo, static bool, rules ...contract.Rule) *contract.Report {
	t.Helper()
	return contract.Audit(context.Background(), cfg, repo, contract.Options{Rules: rules, Static: static})
}

func assertCalled(t *testing.T, r *runner, want ...string) {
	t.Helper()
	if !slices.Equal(r.called, want) {
		t.Errorf("parts ran in order %q, want %q", r.called, want)
	}
}

// seen is a stand-in last check that reports what the earlier results
// hold for each part named, as one finding per part: "<id>/<kind>:
// <result> <first finding>" or "<id>/<kind>: absent".
func seen(parts ...string) func(contract.Earlier) contract.PartResult {
	return func(earlier contract.Earlier) contract.PartResult {
		var out []contract.Finding
		for _, p := range parts {
			id, kind, _ := strings.Cut(p, "/")
			got, ok := earlier.Result(id, contract.PartKind(kind))
			switch {
			case !ok:
				out = append(out, contract.Finding{Message: p + ": absent"})
			case len(got.Findings) > 0:
				out = append(out, contract.Finding{Message: p + ": " + string(got.Result) + " " + got.Findings[0].Message})
			default:
				out = append(out, contract.Finding{Message: p + ": " + string(got.Result)})
			}
		}
		return contract.PartResult{Result: contract.ResultPass, Findings: out}
	}
}

// executionFindings lists the messages of a rule's execution part's findings.
func executionFindings(t *testing.T, rep *contract.Report, id string) []string {
	t.Helper()
	var out []string
	for _, f := range part(t, rep, id, contract.Execution).Findings {
		out = append(out, f.Message)
	}
	return out
}

func TestPartsRunInRuleListOrder(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	two := contract.Rule{ID: "T-02", Parts: []contract.Part{
		r.part("T-02", contract.Execution),
		r.part("T-02", contract.Probe),
	}}
	runAudit(t, cfg, repo, false, r.rule("T-03", contract.Probe), two, r.rule("T-01", contract.Execution))
	assertCalled(t, r, "T-03/probe", "T-02/execution", "T-02/probe", "T-01/execution")
}

// The last part runs after every other execution and probe part, even
// when its rule comes first, and sees all their results.
func TestLastPartRunsLastAndSeesEveryEarlierResult(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	rep := runAudit(t, cfg, repo, false,
		r.lastRule("T-01", contract.Execution, seen("T-02/probe", "T-03/execution", "T-01/execution")),
		r.rule("T-02", contract.Probe),
		r.rule("T-03", contract.Execution),
	)
	assertCalled(t, r, "T-02/probe", "T-03/execution", "T-01/execution")
	want := []string{"T-02/probe: fail from T-02", "T-03/execution: fail from T-03", "T-01/execution: absent"}
	if got := executionFindings(t, rep, "T-01"); !slices.Equal(got, want) {
		t.Errorf("T-01 saw %q, want %q", got, want)
	}
}

// A-02 is not applicable without Claude Code (spec 0053), so the last
// part runs without its probe's result, as R-01 does.
func TestPartOfANotApplicableRuleIsAbsentFromEarlier(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	a02, ok := contract.LookupRule("A-02")
	if !ok {
		t.Fatal("no A-02 in the registry")
	}
	for j := range a02.Parts {
		a02.Parts[j] = r.part("A-02", a02.Parts[j].Kind)
	}
	rep := runAudit(t, cfg, repo, false,
		r.lastRule("T-01", contract.Execution, seen("A-02/probe")),
		a02,
	)
	assertCalled(t, r, "T-01/execution")
	if got, want := executionFindings(t, rep, "T-01"), []string{"A-02/probe: absent"}; !slices.Equal(got, want) {
		t.Errorf("T-01 saw %q, want %q", got, want)
	}
}

// A waived rule's parts do not run, so the last part does not see them.
func TestPartOfAWaivedRuleIsAbsentFromEarlier(t *testing.T) {
	cfg, repo := runnerFixture(t)
	cfg.Waivers = append(cfg.Waivers, contract.Waiver{Rule: "T-02", Reason: "stand-in", Until: time.Now().AddDate(1, 0, 0)})
	r := &runner{}
	waived := r.rule("T-02", contract.Probe)
	waived.Level = contract.Should
	rep := runAudit(t, cfg, repo, false,
		r.lastRule("T-01", contract.Execution, seen("T-02/probe", "T-03/probe")),
		waived,
		r.rule("T-03", contract.Probe),
	)
	assertCalled(t, r, "T-03/probe", "T-01/execution")
	if got, want := executionFindings(t, rep, "T-01"), []string{"T-02/probe: absent", "T-03/probe: fail from T-03"}; !slices.Equal(got, want) {
		t.Errorf("T-01 saw %q, want %q", got, want)
	}
}

// panicText runs f and returns what it panicked with, failing the test
// when it does not panic.
func panicText(t *testing.T, f func()) (text string) {
	t.Helper()
	defer func() {
		v := recover()
		if v == nil {
			t.Fatal("the audit did not panic")
		}
		text = fmt.Sprint(v)
	}()
	f()
	return ""
}

func assertContains(t *testing.T, text string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(text, p) {
			t.Errorf("%q does not name %q", text, p)
		}
	}
}

// pass is a stand-in last check that decides nothing.
func pass(contract.Earlier) contract.PartResult { return contract.Pass() }

// invalidRuleLists are rule lists the runner refuses, each with the text
// its panic must hold.
var invalidRuleLists = []struct {
	name  string
	rules func(r *runner) []contract.Rule
	want  []string
}{
	{
		name: "two last parts",
		rules: func(r *runner) []contract.Rule {
			return []contract.Rule{
				r.lastRule("T-01", contract.Execution, pass),
				r.rule("T-02", contract.Probe),
				r.lastRule("T-03", contract.Probe, pass),
			}
		},
		want: []string{"T-01/execution", "T-03/probe", "last part"},
	},
	{
		name: "a static last part",
		rules: func(r *runner) []contract.Rule {
			return []contract.Rule{r.rule("T-02", contract.Probe), r.lastRule("T-01", contract.Static, pass)}
		},
		want: []string{"T-01/static", "static", "last part"},
	},
	{
		name: "a part with both Run and RunLast",
		rules: func(r *runner) []contract.Rule {
			both := r.last("T-01", contract.Execution, pass)
			both.Run = r.part("T-01", contract.Execution).Run
			return []contract.Rule{r.rule("T-02", contract.Probe), {ID: "T-01", Parts: []contract.Part{both}}}
		},
		want: []string{"T-01/execution", "both Run and RunLast"},
	},
	{
		name: "a part with neither Run nor RunLast",
		rules: func(r *runner) []contract.Rule {
			return []contract.Rule{r.rule("T-02", contract.Probe), {ID: "T-01", Parts: []contract.Part{{Kind: contract.Static}}}}
		},
		want: []string{"T-01/static", "neither Run nor RunLast"},
	},
}

func TestInvalidRuleListPanicsBeforeTheProbeCopy(t *testing.T) {
	cfg, repo := runnerFixture(t)
	for _, tc := range invalidRuleLists {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{}
			probe, stderr := keepingCopy(t)
			text := panicText(t, func() {
				contract.Audit(context.Background(), cfg, repo, contract.Options{Rules: tc.rules(r), Probe: probe})
			})
			assertContains(t, text, append([]string{"contract: "}, tc.want...)...)
			assertCalled(t, r)
			if kept := stderr.String(); kept != "" {
				t.Errorf("the audit built a probe copy before it panicked: %q", kept)
			}
		})
	}
}

func TestStaticAuditValidatesTheRuleListAndRunsNoPart(t *testing.T) {
	cfg, repo := runnerFixture(t)
	for _, tc := range invalidRuleLists {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{}
			text := panicText(t, func() { runAudit(t, cfg, repo, true, tc.rules(r)...) })
			assertContains(t, text, tc.want...)
			assertCalled(t, r)
		})
	}

	rules := func(r *runner) []contract.Rule {
		return []contract.Rule{
			r.lastRule("T-01", contract.Execution, pass),
			r.rule("T-02", contract.Probe),
		}
	}
	r := &runner{}
	rep, kept := auditKeepingCopy(t, cfg, repo, true, rules(r))
	assertCalled(t, r)
	for _, p := range []struct {
		id   string
		kind contract.PartKind
	}{{"T-01", contract.Execution}, {"T-02", contract.Probe}} {
		if got := part(t, rep, p.id, p.kind).Result; got != contract.ResultNotRun {
			t.Errorf("%s/%s is %s, want %s", p.id, p.kind, got, contract.ResultNotRun)
		}
	}
	if kept != "" {
		t.Errorf("the static audit built a probe copy: %q", kept)
	}

	// The same rules without Static build the copy, so the check above
	// can see one.
	if _, kept := auditKeepingCopy(t, cfg, repo, false, rules(&runner{})); !strings.Contains(kept, "keeping the probe copy at") {
		t.Errorf("the audit without Static kept no probe copy: %q", kept)
	}
}

// auditKeepingCopy runs rules with ProbeOptions.KeepCopy, in a temporary
// directory of its own, and returns the report and what the audit wrote
// on Stderr: the kept copy's path, or nothing when it built no copy.
func auditKeepingCopy(t *testing.T, cfg *contract.Config, repo *contract.Repo, static bool, rules []contract.Rule) (*contract.Report, string) {
	t.Helper()
	probe, stderr := keepingCopy(t)
	rep := contract.Audit(context.Background(), cfg, repo, contract.Options{Rules: rules, Static: static, Probe: probe})
	return rep, stderr.String()
}

// keepingCopy returns probe options that keep the probe copy, in a
// temporary directory of the test's own, and say so on the returned
// Stderr; it stays empty when the audit builds no copy.
func keepingCopy(t *testing.T) (contract.ProbeOptions, *strings.Builder) {
	t.Helper()
	tmp := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", tmp)
		t.Setenv("TEMP", tmp)
	} else {
		t.Setenv("TMPDIR", tmp)
	}
	stderr := &strings.Builder{}
	return contract.ProbeOptions{KeepCopy: true, Stderr: stderr}, stderr
}
