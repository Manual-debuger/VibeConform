package contract_test

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// The runner orders the execution and probe parts by the needs they
// declare (spec 0064). These tests run contract.Audit over stand-in
// rules in Options.Rules and read the order in which the stand-ins were
// called and what each one saw.

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

// part is a stand-in part that records its call and runs check, or
// fails with "from <id>" when check is nil.
func (r *runner) part(id string, kind contract.PartKind, check func(contract.Needed) contract.PartResult, needs ...contract.Need) contract.Part {
	return contract.Part{Kind: kind, Needs: needs, Run: func(_ *contract.Env, needed contract.Needed) contract.PartResult {
		r.called = append(r.called, partName(id, kind))
		if check == nil {
			return contract.Fail(contract.Finding{Message: "from " + id})
		}
		return check(needed)
	}}
}

// rule is a stand-in rule of one part.
func (r *runner) rule(id string, kind contract.PartKind, check func(contract.Needed) contract.PartResult, needs ...contract.Need) contract.Rule {
	return contract.Rule{ID: id, Parts: []contract.Part{r.part(id, kind, check, needs...)}}
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

// seen is a stand-in check that reports what its view holds for each
// part named, as one finding per part: "<id>/<kind>: <result> <first
// finding>" or "<id>/<kind>: absent".
func seen(parts ...string) func(contract.Needed) contract.PartResult {
	return func(needed contract.Needed) contract.PartResult {
		var out []contract.Finding
		for _, p := range parts {
			id, kind, _ := strings.Cut(p, "/")
			got, ok := needed.Result(id, contract.PartKind(kind))
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

func TestPartRunsAfterThePartItNeeds(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	rep := runAudit(t, cfg, repo, false,
		r.rule("T-02", contract.Execution, seen("T-01/probe"), contract.NeedPart("T-01", contract.Probe)),
		r.rule("T-01", contract.Probe, nil),
	)
	assertCalled(t, r, "T-01/probe", "T-02/execution")
	if got, want := executionFindings(t, rep, "T-02"), []string{"T-01/probe: fail from T-01"}; !slices.Equal(got, want) {
		t.Errorf("T-02 saw %q, want %q", got, want)
	}
}

func TestPartsWithoutNeedsRunInRuleListOrder(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	two := contract.Rule{ID: "T-02", Parts: []contract.Part{
		r.part("T-02", contract.Execution, nil),
		r.part("T-02", contract.Probe, nil),
	}}
	runAudit(t, cfg, repo, false, r.rule("T-03", contract.Probe, nil), two, r.rule("T-01", contract.Execution, nil))
	assertCalled(t, r, "T-03/probe", "T-02/execution", "T-02/probe", "T-01/execution")
}

func TestPartThatNeedsEveryOtherPartRunsLastAndSeesThemAll(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	rep := runAudit(t, cfg, repo, false,
		r.rule("T-01", contract.Execution, seen("T-02/probe", "T-03/execution"), contract.NeedEveryOther()),
		r.rule("T-02", contract.Probe, nil),
		r.rule("T-03", contract.Execution, nil),
	)
	assertCalled(t, r, "T-02/probe", "T-03/execution", "T-01/execution")
	want := []string{"T-02/probe: fail from T-02", "T-03/execution: fail from T-03"}
	if got := executionFindings(t, rep, "T-01"); !slices.Equal(got, want) {
		t.Errorf("T-01 saw %q, want %q", got, want)
	}
}

// A-02 is not applicable without Claude Code (spec 0053), so a part that
// needs its probe runs without it, as R-01 does.
func TestNeededPartOfANotApplicableRuleIsAbsent(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	a02, ok := contract.LookupRule("A-02")
	if !ok {
		t.Fatal("no A-02 in the registry")
	}
	for j := range a02.Parts {
		a02.Parts[j] = r.part("A-02", a02.Parts[j].Kind, nil)
	}
	rep := runAudit(t, cfg, repo, false,
		r.rule("T-01", contract.Execution, seen("A-02/probe"), contract.NeedPart("A-02", contract.Probe)),
		a02,
	)
	assertCalled(t, r, "T-01/execution")
	if got, want := executionFindings(t, rep, "T-01"), []string{"A-02/probe: absent"}; !slices.Equal(got, want) {
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

func TestInvalidNeedsPanicBeforeTheProbeCopy(t *testing.T) {
	cfg, repo := runnerFixture(t)
	const notInList = "which is not in the rule list"
	for _, tc := range []struct {
		name  string
		rules func(r *runner) []contract.Rule
		want  []string
		// not is text the message must not hold.
		not []string
	}{
		{
			name: "a need on a part not in the rule list",
			rules: func(r *runner) []contract.Rule {
				return []contract.Rule{r.rule("T-01", contract.Execution, nil, contract.NeedPart("T-02", contract.Probe))}
			},
			want: []string{"contract: T-01/execution needs T-02/probe, which is not in the rule list"},
		},
		{
			name: "a need on a kind the rule does not have",
			rules: func(r *runner) []contract.Rule {
				return []contract.Rule{
					r.rule("T-01", contract.Execution, nil, contract.NeedPart("T-02", contract.Probe)),
					r.rule("T-02", contract.Execution, nil),
				}
			},
			want: []string{"contract: T-01/execution needs T-02/probe, which is not in the rule list"},
		},
		{
			name: "two parts that each need every other part",
			rules: func(r *runner) []contract.Rule {
				return []contract.Rule{
					r.rule("T-01", contract.Execution, nil, contract.NeedEveryOther()),
					r.rule("T-02", contract.Probe, nil, contract.NeedEveryOther()),
				}
			},
			want: []string{"T-01/execution", "T-02/probe", "every other execution and probe part", "cycle"},
		},
		{
			name: "two parts that need each other",
			rules: func(r *runner) []contract.Rule {
				return []contract.Rule{
					r.rule("T-01", contract.Execution, nil, contract.NeedPart("T-02", contract.Probe)),
					r.rule("T-02", contract.Probe, nil, contract.NeedPart("T-01", contract.Execution)),
				}
			},
			want: []string{"cycle", "T-01/execution needs T-02/probe", "T-02/probe needs T-01/execution"},
		},
		{
			name: "a static part with a need",
			rules: func(r *runner) []contract.Rule {
				return []contract.Rule{
					r.rule("T-01", contract.Static, nil, contract.NeedPart("T-02", contract.Probe)),
					r.rule("T-02", contract.Probe, nil),
				}
			},
			want: []string{"contract: T-01/static needs T-02/probe", "static"},
			not:  []string{notInList},
		},
		{
			name: "a static part with a need on a part not in the rule list",
			rules: func(r *runner) []contract.Rule {
				return []contract.Rule{r.rule("T-01", contract.Static, nil, contract.NeedPart("T-02", contract.Probe))}
			},
			want: []string{"contract: T-01/static needs T-02/probe", "a static part declares no needs"},
			not:  []string{notInList},
		},
		{
			name: "a need on a static part in the rule list",
			rules: func(r *runner) []contract.Rule {
				return []contract.Rule{
					r.rule("T-01", contract.Execution, nil, contract.NeedPart("T-02", contract.Static)),
					r.rule("T-02", contract.Static, nil),
				}
			},
			want: []string{"contract: T-01/execution needs T-02/static", "a static part"},
			not:  []string{notInList},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{}
			text := panicText(t, func() { runAudit(t, cfg, repo, false, tc.rules(r)...) })
			assertContains(t, text, tc.want...)
			for _, n := range tc.not {
				if strings.Contains(text, n) {
					t.Errorf("%q says %q", text, n)
				}
			}
			assertCalled(t, r)
		})
	}
}

func TestStaticAuditValidatesNeedsAndRunsNoPart(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	text := panicText(t, func() {
		runAudit(t, cfg, repo, true, r.rule("T-01", contract.Execution, nil, contract.NeedPart("T-02", contract.Probe)))
	})
	assertContains(t, text, "T-01/execution needs T-02/probe")

	rules := func(r *runner) []contract.Rule {
		return []contract.Rule{
			r.rule("T-01", contract.Execution, nil, contract.NeedPart("T-02", contract.Probe)),
			r.rule("T-02", contract.Probe, nil),
		}
	}
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
	tmp := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", tmp)
		t.Setenv("TEMP", tmp)
	} else {
		t.Setenv("TMPDIR", tmp)
	}
	var stderr strings.Builder
	rep := contract.Audit(context.Background(), cfg, repo, contract.Options{
		Rules: rules, Static: static, Probe: contract.ProbeOptions{KeepCopy: true, Stderr: &stderr},
	})
	return rep, stderr.String()
}

func TestPartSeesOnlyItsDeclaredNeeds(t *testing.T) {
	cfg, repo := runnerFixture(t)
	r := &runner{}
	rep := runAudit(t, cfg, repo, false,
		r.rule("T-01", contract.Probe, nil),
		r.rule("T-02", contract.Probe, nil),
		r.rule("T-03", contract.Execution, seen("T-01/probe", "T-02/probe", "T-03/execution"), contract.NeedPart("T-02", contract.Probe)),
	)
	want := []string{"T-01/probe: absent", "T-02/probe: fail from T-02", "T-03/execution: absent"}
	if got := executionFindings(t, rep, "T-03"); !slices.Equal(got, want) {
		t.Errorf("T-03 saw %q, want %q", got, want)
	}
}
