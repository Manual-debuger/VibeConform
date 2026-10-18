package contract

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The guard probes of #103 (spec 0053, "The rules"): A-02 feeds the
// contract's guard corpus to every PreToolUse hook that matches a case's
// tool; A-03 feeds every deny case again with PATH reduced to the
// operating system's directories and with policy.json replaced by
// invalid JSON. A hook refuses only by exiting 2.

// GuardExceptionUse is one corpus case a guard exception let through
// (A-02), listed in the report under its exception.
type GuardExceptionUse struct {
	Rule  string
	Match string
	Case  string
}

// GuardCorpusCount is the number of deny cases of one guard rule ID.
type GuardCorpusCount struct {
	Rule string
	Deny int
}

// GuardCorpusCounts returns the corpus's deny cases by guard rule ID, in
// the order of the IDs, and its deny and allow totals. --explain A-02 and
// A-03 print them.
func GuardCorpusCounts() (byRule []GuardCorpusCount, deny, allow int) {
	n := map[string]int{}
	for _, c := range guardCorpus() {
		if c.Want == WantDeny {
			n[c.Rule]++
			deny++
		} else {
			allow++
		}
	}
	for _, id := range guardrule.IDs {
		byRule = append(byRule, GuardCorpusCount{Rule: id, Deny: n[id]})
	}
	return byRule, deny, allow
}

// preToolUseHooks reads the PreToolUse command hooks. When it cannot,
// ok is false and result is what the probe decides.
func preToolUseHooks(env *Env) (hooks []settingsHook, result PartResult, ok bool) {
	settings, res, ok := env.readClaudeSettings("add a PreToolUse hook that runs the guard")
	if !ok {
		return nil, res, false
	}
	return settings.commandHooks(eventPreToolUse), Pass(), true
}

// hookRun is one hook's run for one case.
type hookRun struct {
	command string
	out     runOutcome
}

// caseRun is one corpus case fed to every hook that matches its tool.
type caseRun struct {
	c    GuardCase
	runs []hookRun
}

// refused reports whether some hook exited 2.
func (r caseRun) refused() bool {
	for _, h := range r.runs {
		if h.out.Ran() && h.out.ExitCode == 2 {
			return true
		}
	}
	return false
}

// undecided returns the first run that did not end by itself, when no
// hook refused.
func (r caseRun) undecided() *runOutcome {
	for _, h := range r.runs {
		if !h.out.Ran() {
			return &h.out
		}
	}
	return nil
}

// what names the case: its name, tool and command or path.
func (r caseRun) what() string { return describeGuardCase(r.c) }

func describeGuardCase(c GuardCase) string {
	t := c.Target()
	arg := t.Command
	if guardrule.IsEditTool(t.Tool) {
		arg = t.Path
	}
	return fmt.Sprintf("corpus case %q (%s %q)", c.Name, c.Payload.ToolName, arg)
}

// exits says how every hook ended.
func (r caseRun) exits() string {
	if len(r.runs) == 0 {
		return "no PreToolUse hook matches " + r.c.Payload.ToolName
	}
	var parts []string
	for _, h := range r.runs {
		if h.out.Ran() {
			parts = append(parts, fmt.Sprintf("hook %q exited %d", shortCommand(h.command), h.out.ExitCode))
		} else {
			parts = append(parts, fmt.Sprintf("hook %q: %s", shortCommand(h.command), h.out.Undecided))
		}
	}
	return strings.Join(parts, "; ")
}

// refusal is the first line a refusing hook printed, or "".
func (r caseRun) refusal() string {
	for _, h := range r.runs {
		if h.out.Ran() && h.out.ExitCode == 2 {
			line, _, _ := strings.Cut(strings.TrimSpace(strings.ReplaceAll(string(h.out.Output), "\r\n", "\n")), "\n")
			return line
		}
	}
	return ""
}

// shortCommand shortens a long hook command for a finding.
func shortCommand(c string) string {
	const limit = 60
	if r := []rune(c); len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return c
}

// maxParallelCases bounds how many corpus cases run at once. A hook run
// is a bash and a guard process; on Windows each costs about 0.2 s, and
// one by one the corpus would take minutes.
var maxParallelCases = min(runtime.NumCPU(), 8)

// feedCases runs each corpus case through every hook that matches its
// tool, with the unlock marker in the state the case names, and returns
// the runs in the order of cases. Cases with the same unlock state run
// concurrently; the states run one after another, because the marker is
// one file. The R-01 shim's calls during these runs are recorded
// afterwards, under who.
func feedCases(s *probeSession, hooks []settingsHook, cases []GuardCase, who string, label func(GuardCase) string, o RunOpts) ([]caseRun, error) {
	runs := make([]caseRun, len(cases))
	defer func() {
		out := runOutcome{Label: who, File: claudeSettingsFile}
		s.recordShimCalls(&out)
	}()
	for _, state := range []string{UnlockNone, UnlockExpired, UnlockValid} {
		var idx []int
		for i, c := range cases {
			if c.Unlock == state {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			continue
		}
		if err := s.setUnlock(state); err != nil {
			return nil, err
		}
		work := make(chan int)
		var wg sync.WaitGroup
		for range min(maxParallelCases, len(idx)) {
			wg.Go(func() {
				for i := range work {
					runs[i] = feedCase(s, hooks, cases[i], label(cases[i]), o)
				}
			})
		}
		for _, i := range idx {
			work <- i
		}
		close(work)
		wg.Wait()
	}
	return runs, nil
}

// feedCase runs one case through every hook that matches its tool, one
// hook after another. It may run concurrently with other cases.
func feedCase(s *probeSession, hooks []settingsHook, c GuardCase, label string, o RunOpts) caseRun {
	run := caseRun{c: c}
	var payload []byte
	for _, h := range hooks {
		if !h.entry.matches(c.Payload.ToolName) {
			continue
		}
		if payload == nil {
			payload = s.HookPayload(eventPreToolUse, map[string]any{
				"tool_name":  c.Payload.ToolName,
				"tool_input": c.Payload.ToolInput,
			})
		}
		out := s.runHook(label, h.command, h.limit, payload, o)
		run.runs = append(run.runs, hookRun{command: h.command, out: out})
	}
	return run
}

// setUnlock writes the unlock marker into the copy's common Git
// directory for a case's unlock state, or removes it: one RFC 3339 line,
// ten minutes ahead for a valid marker and one minute in the past for an
// expired one (spec 0053, A-08).
func (s *probeSession) setUnlock(state string) error {
	p := filepath.Join(s.GitDir(), guardrule.UnlockMarkerFile)
	var at time.Time
	switch state {
	case UnlockValid:
		at = time.Now().Add(10 * time.Minute)
	case UnlockExpired:
		at = time.Now().Add(-time.Minute)
	default:
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return os.WriteFile(p, guardrule.FormatUnlockMarker(at), 0o600)
}

// policyExceptions returns the guard policy's exceptions as the audit
// applies them: entries with a guard rule ID and a match. Only
// policy.json counts; policy.local.json never does.
func policyExceptions(env *Env) []guardrule.Exception {
	var out []guardrule.Exception
	for _, g := range guardExceptions(env) {
		if guardrule.IsContractID(g.Rule) && g.Match != "" {
			out = append(out, guardrule.Exception{Rule: g.Rule, Match: g.Match, Reason: g.Reason})
		}
	}
	return out
}

// guardFix is the fix for a deny case the guard lets through.
func guardFix(env *Env, rule string) string {
	fix := "make the guard refuse it (exit 2), or list an exception for " + rule + " in " + guardPolicyFile
	if env.Repo.HasFile(localPolicyFile) {
		fix += "; exceptions in the tracked " + localPolicyFile + " do not count"
	}
	return fix
}

// checkA02: every corpus case, fed to every PreToolUse hook that matches
// its tool, is refused (or let through by a policy.json exception) when
// it is a deny case and allowed when it is an allow case. It records its
// deny-case verdict for R-01 and the exceptions' uses for the report.
func checkA02(env *Env) PartResult {
	s := env.probe
	res := a02(env, s)
	if s.guardDeny == nil {
		s.guardDeny = &res
	}
	return res
}

func a02(env *Env, s *probeSession) PartResult {
	hooks, res, ok := preToolUseHooks(env)
	if !ok {
		return res
	}
	if len(hooks) > 0 {
		if ok, why := s.HasBash(); !ok {
			return Undecided(Finding{File: claudeSettingsFile, Message: "cannot decide: " + why})
		}
	}
	defer func() { _ = s.setUnlock(UnlockNone) }()

	exceptions := policyExceptions(env)
	deny, all := &verdicts{}, &verdicts{}
	unmatched := map[string][]string{}
	var tools []string
	runs, err := feedCases(s, hooks, guardCorpus(), "the PreToolUse hooks during A-02",
		func(c GuardCase) string { return "A-02 with " + describeGuardCase(c) }, RunOpts{})
	if err != nil {
		f := Finding{Message: "cannot decide: cannot write the unlock marker: " + err.Error()}
		d := Undecided(f)
		s.guardDeny = &d
		return d
	}
	for _, run := range runs {
		c := run.c
		switch {
		case c.Want == WantDeny && run.refused():
		case c.Want == WantDeny && run.undecided() != nil:
			f := Finding{File: claudeSettingsFile, Message: "cannot decide: " + run.what() + ": " + run.undecided().describe()}
			all.undecided(f)
			deny.undecided(f)
		case c.Want == WantDeny && len(run.runs) == 0 && len(exceptionsFor(exceptions, c, s)) == 0:
			if _, seen := unmatched[c.Payload.ToolName]; !seen {
				tools = append(tools, c.Payload.ToolName)
			}
			unmatched[c.Payload.ToolName] = append(unmatched[c.Payload.ToolName], fmt.Sprintf("%q", c.Name))
		case c.Want == WantDeny:
			if ex := exceptionsFor(exceptions, c, s); len(ex) > 0 {
				env.guardUses = append(env.guardUses, GuardExceptionUse{Rule: ex[0].Rule, Match: ex[0].Match, Case: c.Name})
				continue
			}
			f := Finding{File: claudeSettingsFile,
				Message: fmt.Sprintf("%s should be refused (guard rule %s); %s", run.what(), c.Rule, run.exits()),
				Fix:     guardFix(env, c.Rule)}
			all.fail(f)
			deny.fail(f)
		case run.refused():
			msg := fmt.Sprintf("%s should be allowed; %s", run.what(), run.exits())
			if line := run.refusal(); line != "" {
				msg += ": " + line
			}
			all.fail(Finding{File: claudeSettingsFile, Message: msg,
				Fix: "make the guard allow it: the contract's corpus expects this call allowed"})
		case run.undecided() != nil:
			all.undecided(Finding{File: claudeSettingsFile, Message: "cannot decide: " + run.what() + ": " + run.undecided().describe()})
		}
	}
	for _, tool := range tools {
		f := Finding{File: claudeSettingsFile,
			Message: fmt.Sprintf("no PreToolUse hook matches %s, so its %d deny cases are not refused: %s",
				tool, len(unmatched[tool]), strings.Join(unmatched[tool], ", ")),
			Fix: "add a PreToolUse hook whose matcher matches " + tool + " and runs the guard"}
		all.fail(f)
		deny.fail(f)
	}
	d := deny.result()
	s.guardDeny = &d
	return all.result()
}

// exceptionsFor returns the policy exceptions for a case's rule ID that
// match it.
func exceptionsFor(exceptions []guardrule.Exception, c GuardCase, s *probeSession) []guardrule.Exception {
	var out []guardrule.Exception
	for _, ex := range exceptions {
		if ex.Rule == c.Rule && ex.Matches(c.Target(), s.Root()) {
			out = append(out, ex)
		}
	}
	return out
}

// verdicts collects the failures and undecided runs of a probe. Its
// result fails with every failure, or is undecided with the first
// undecided run and a count of the others.
type verdicts struct {
	fails     []Finding
	undecides []Finding
}

func (v *verdicts) fail(f Finding)      { v.fails = append(v.fails, f) }
func (v *verdicts) undecided(f Finding) { v.undecides = append(v.undecides, f) }

func (v *verdicts) result() PartResult {
	switch {
	case len(v.fails) > 0:
		return Fail(v.fails...)
	case len(v.undecides) > 0:
		f := v.undecides[0]
		if n := len(v.undecides) - 1; n > 0 {
			f.Message += fmt.Sprintf(" (and %d more %s)", n, plural(n, "case", "cases"))
		}
		return Undecided(f)
	}
	return Pass()
}

// a03Situation is one situation in which the guard cannot work as
// configured, and every deny case must still be refused.
type a03Situation struct {
	what string
	fix  string
	// prepare changes the copy (or the run) and returns how to undo it.
	prepare func(s *probeSession) (RunOpts, func() error, error)
}

var a03Situations = []a03Situation{
	{
		what: "with PATH reduced to the operating system's directories",
		fix: "make each PreToolUse hook exit 2 when its guard program is not on PATH, such as " +
			"`command -v vibe-guard >/dev/null 2>&1 || exit 2` before it runs",
		prepare: func(s *probeSession) (RunOpts, func() error, error) {
			return RunOpts{Env: envSet(s.runEnv, "PATH", systemPath(s.runEnv))}, func() error { return nil }, nil
		},
	},
	{
		what: "with " + guardPolicyFile + " replaced by `{`",
		fix:  "make the guard refuse every call (exit 2) when " + guardPolicyFile + " does not parse",
		prepare: func(s *probeSession) (RunOpts, func() error, error) {
			p := filepath.Join(s.Root(), filepath.FromSlash(guardPolicyFile))
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				return RunOpts{}, nil, err
			}
			if err := os.WriteFile(p, []byte("{"), 0o600); err != nil {
				return RunOpts{}, nil, err
			}
			return RunOpts{}, s.Reset, nil
		},
	},
}

// systemPath is the operating system's directories as a PATH (spec
// 0053, A-03).
func systemPath(env []string) string {
	if runtime.GOOS != "windows" {
		return "/usr/bin:/bin:/usr/sbin:/sbin"
	}
	root, _ := envGet(env, "SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	dirs := []string{
		filepath.Join(root, "System32"),
		root,
		filepath.Join(root, "System32", "Wbem"),
		filepath.Join(root, "System32", "WindowsPowerShell", "v1.0"),
		filepath.Join(root, "System32", "OpenSSH"),
	}
	return strings.Join(dirs, ";")
}

// checkA03: every deny case is refused in each situation of
// a03Situations. Exceptions do not count.
func checkA03(env *Env) PartResult {
	s := env.probe
	hooks, res, ok := preToolUseHooks(env)
	if !ok {
		return res
	}
	if len(hooks) > 0 {
		if ok, why := s.HasBash(); !ok {
			return Undecided(Finding{File: claudeSettingsFile, Message: "cannot decide: " + why})
		}
	}
	defer func() { _ = s.setUnlock(UnlockNone) }()

	v := &verdicts{}
	for _, sit := range a03Situations {
		a03Run(s, hooks, sit, v)
	}
	return v.result()
}

func a03Run(s *probeSession, hooks []settingsHook, sit a03Situation, v *verdicts) {
	o, undo, err := sit.prepare(s)
	if err != nil {
		v.undecided(Finding{Message: "cannot decide: cannot prepare the probe copy " + sit.what + ": " + err.Error()})
		return
	}
	defer func() {
		if err := undo(); err != nil {
			v.undecided(Finding{Message: "cannot decide: cannot restore the probe copy: " + err.Error()})
		}
	}()
	var deny []GuardCase
	for _, c := range guardCorpus() {
		if c.Want == WantDeny {
			deny = append(deny, c)
		}
	}
	runs, err := feedCases(s, hooks, deny, "the PreToolUse hooks during A-03",
		func(c GuardCase) string { return "A-03 " + sit.what + ": " + describeGuardCase(c) }, o)
	if err != nil {
		v.undecided(Finding{Message: "cannot decide: cannot write the unlock marker: " + err.Error()})
		return
	}
	var missed []string
	for _, run := range runs {
		switch {
		case run.refused():
		case run.undecided() != nil:
			v.undecided(Finding{File: claudeSettingsFile,
				Message: "cannot decide: " + sit.what + ", " + run.what() + ": " + run.undecided().describe()})
		default:
			missed = append(missed, run.what()+": "+run.exits())
		}
	}
	if len(missed) > 0 {
		v.fail(Finding{File: claudeSettingsFile,
			Message: fmt.Sprintf("%s, %d deny %s not refused:\n%s", sit.what, len(missed),
				plural(len(missed), "case was", "cases were"), strings.Join(missed, "\n")),
			Fix: sit.fix})
	}
}
