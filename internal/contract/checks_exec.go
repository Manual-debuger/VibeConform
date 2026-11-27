package contract

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The execution parts of #102 (spec 0053, "The rules"): V-03, A-07's
// execution part, D-01 and R-01's execution part. They run in the probe
// copy, which the audit resets before each part.

// undecidedRun is the finding for a run that could not run or end by
// itself.
func undecidedRun(out runOutcome) PartResult {
	return undecided(Finding{File: out.File, Message: "cannot decide: " + out.describe()})
}

// mergeRuns combines the results of several runs of one part, as merge
// does. When one fails, only the failures' findings are kept: the part
// fails whatever the undecided runs would have shown, and each failure
// names its file and fix.
func mergeRuns(results ...PartResult) PartResult {
	out := merge(results...)
	if out.Result != ResultFail {
		return out
	}
	out.Findings = nil
	for _, r := range results {
		if r.Result == ResultFail {
			out.Findings = append(out.Findings, r.Findings...)
		}
	}
	return out
}

// checkV03: with the format defect planted in each component, fmt-check
// leaves only the planted file in git status, unchanged.
func checkV03(env *Env) PartResult {
	if f := needEntrypoint(env, entrypointFmtCheck); f != nil {
		return Fail(*f)
	}
	s := env.probe
	var results []PartResult
	for _, comp := range env.config.Components {
		results = append(results, v03Component(env, s, comp))
	}
	return mergeRuns(results...)
}

func v03Component(env *Env, s *probeSession, comp Component) PartResult {
	if err := s.Reset(); err != nil {
		return undecided(Finding{Message: "cannot decide: cannot reset the probe copy: " + err.Error()})
	}
	d, err := defectFor(env, comp, defectFormat)
	if err != nil {
		return undecided(Finding{Message: "cannot decide: nowhere to plant the format defect: " + err.Error()})
	}
	if err := s.Plant(d.Path, d.Content); err != nil {
		return undecided(Finding{Message: "cannot decide: cannot plant " + d.Path + ": " + err.Error()})
	}
	run := s.RunEntrypoint(env, entrypointFmtCheck, "fmt-check with the format defect in "+comp.Path)
	if run.Missing != nil {
		return Fail(*run.Missing)
	}
	if !run.Ran() {
		return undecidedRun(run.runOutcome)
	}
	status, err := s.Status()
	if err != nil {
		return undecided(Finding{Message: "cannot decide: " + err.Error()})
	}
	var problems []string
	for _, e := range status {
		if e != "?? "+d.Path {
			problems = append(problems, e)
		}
	}
	if got, err := s.ReadFile(d.Path); err != nil {
		problems = append(problems, d.Path+" (the planted file is gone)")
	} else if !bytes.Equal(got, d.Content) {
		problems = append(problems, d.Path+" (the planted file was changed)")
	}
	if len(problems) == 0 {
		return Pass()
	}
	return Fail(Finding{
		File: env.config.file,
		Message: fmt.Sprintf("fmt-check (%s) wrote files in component %s; git status lists: %s",
			run.Command, comp.Path, strings.Join(problems, ", ")),
		Fix: "make entrypoints.fmt-check only check formatting, such as gofmt -l or prettier --check; the command that rewrites files belongs in entrypoints.fmt",
	})
}

// checkA07Execution: every SessionStart hook that matches startup exits
// 0 in the reset copy and leaves git status empty.
func checkA07Execution(env *Env) PartResult {
	if !env.repo.HasFile(claudeSettingsFile) {
		// The static part fails for it.
		return Pass()
	}
	settings, res, ok := env.readClaudeSettings("add a SessionStart hook")
	if !ok {
		return res
	}
	s := env.probe
	var results []PartResult
	for _, h := range settings.commandHooks(eventSessionStart) {
		if h.entry.matches("startup") {
			results = append(results, a07Hook(s, h.command, h.limit))
		}
	}
	return mergeRuns(results...)
}

func a07Hook(s *probeSession, command string, limit time.Duration) PartResult {
	if err := s.Reset(); err != nil {
		return undecided(Finding{Message: "cannot decide: cannot reset the probe copy: " + err.Error()})
	}
	payload := s.HookPayload(eventSessionStart, map[string]any{"source": "startup"})
	out := s.RunHook("the SessionStart hook", command, limit, payload, runOpts{})
	if !out.Ran() {
		return undecidedRun(out)
	}
	if out.ExitCode != 0 {
		return Fail(Finding{File: claudeSettingsFile, Message: out.describe(),
			Fix: "make the SessionStart hook exit 0; it only injects facts"})
	}
	status, err := s.Status()
	if err != nil {
		return undecided(Finding{Message: "cannot decide: " + err.Error()})
	}
	if len(status) > 0 {
		return Fail(Finding{File: claudeSettingsFile,
			Message: fmt.Sprintf("the SessionStart hook (%s) changed files; git status lists: %s", command, strings.Join(status, ", ")),
			Fix:     "make the SessionStart hook write nothing in the repository; print facts to stdout instead"})
	}
	return Pass()
}

// checkR01Execution: with vibe replaced by the shim, the baseline verify
// passes and the A-02 deny cases are still refused. The findings list
// every call of vibe the shim recorded in any run of the audit: setup's,
// those of every earlier part, and those of R-01's own runs.
func checkR01Execution(env *Env, earlier Earlier) PartResult {
	if f := needEntrypoint(env, entrypointVerify); f != nil {
		return Fail(*f)
	}
	s := env.probe
	var verify PartResult
	run, err := s.Baseline(env, entrypointVerify)
	switch {
	case err != nil:
		verify = undecided(Finding{Message: "cannot decide: cannot reset the probe copy: " + err.Error()})
	case run.Missing != nil:
		verify = Fail(*run.Missing)
	case !run.Ran():
		verify = undecidedRun(run.runOutcome)
	case run.ExitCode != 0:
		verify = Fail(Finding{File: env.config.file,
			Message: "with vibe not available, " + run.describe(),
			Fix:     "make entrypoints.verify work without vibe; only the audit job may run vibe"})
	default:
		verify = Pass()
	}

	res := mergeRuns(verify, r01GuardHalf(env, earlier))
	for _, c := range append(earlier.vibeCalls(), run.ShimCalls...) {
		res.Findings = append(res.Findings, Finding{
			File:    c.File,
			Message: fmt.Sprintf("%s ran %q, and vibe was not available", c.Run, c.String()),
			Fix:     "remove the call of vibe from " + c.Run + "; only the audit job may run vibe",
		})
	}
	return res
}

// r01GuardHalf is R-01's second half: the A-02 deny cases are still
// refused with the shim in place. A-02's probe (#103) runs every case
// with the shim on PATH and returns its deny-case verdict; this half
// reads it from the earlier results. Without Claude Code there is no
// guard to check. Where A-02 does not count in the verdict (under 2.1.0,
// when the guard set is not required), a MUST rule does not hold the
// repository to the guard either (spec 0074).
func r01GuardHalf(env *Env, earlier Earlier) PartResult {
	if ok, _ := needsClaude(env.config); !ok {
		return Pass()
	}
	if !env.guardSetBinds() {
		return Pass()
	}
	if g, ok := a02Deny(earlier); ok {
		if g.Result == ResultPass {
			return Pass()
		}
		return g
	}
	return undecided(Finding{Message: "cannot decide whether the A-02 deny cases are refused without vibe: A-02's probe did not run"})
}

// a02Deny returns A-02's verdict on its deny cases from the earlier
// results, or false when A-02's probe did not run.
func a02Deny(earlier Earlier) (PartResult, bool) {
	r, ok := earlier.Result("A-02", Probe)
	if !ok || r.deny == nil {
		return PartResult{}, false
	}
	return *r.deny, true
}

// d01CheckText is the check text of D-01's execution part. Its
// commands come from the lockfile records, so it names what runs.
var d01CheckText = "per component, by the lockfile found. " + lockfileCheckText() +
	" A component with dependencies and no recognized lockfile fails."

// checkD01: each component's lockfiles match its manifest, and a
// component with dependencies has a lockfile.
func checkD01(env *Env) PartResult {
	var results []PartResult
	for _, comp := range env.config.Components {
		results = append(results, d01Component(env, comp))
	}
	return mergeRuns(results...)
}

func d01Component(env *Env, comp Component) PartResult {
	s := env.probe
	var results []PartResult
	found := false
	for _, lf := range allLockfiles() {
		file := joinRel(comp.Path, lf.file)
		if !env.repo.HasFile(file) {
			continue
		}
		found = true
		if err := s.Reset(); err != nil {
			return undecided(Finding{Message: "cannot decide: cannot reset the probe copy: " + err.Error()})
		}
		results = append(results, d01Lockfile(s, comp, file, lf))
	}
	if found {
		return mergeRuns(results...)
	}

	manifest, deps, err := hasDependencies(env, comp)
	switch {
	case errors.Is(err, errNoManifest):
		return Pass()
	case err != nil:
		return Fail(parseFailure(manifest, err))
	case deps:
		names := comp.language().lockfileNames()
		return Fail(Finding{File: manifest,
			Message: fmt.Sprintf("component %s has dependencies and no recognized lockfile (%s)", comp.Path, names),
			Fix:     "commit the lockfile your package manager writes, such as " + names})
	}
	return Pass()
}

// d01Lockfile runs a lockfile's check commands in the component's
// directory; file is the lockfile's path from the repository root.
func d01Lockfile(s *probeSession, comp Component, file string, lf lockfile) PartResult {
	for _, argv := range lf.check {
		label := strings.Join(argv, " ") + " in " + comp.Path
		out := s.RunProgram(label, file, argv, runOpts{Dir: comp.Path})
		if !out.Ran() {
			return undecidedRun(out)
		}
		if out.ExitCode != 0 {
			return Fail(Finding{File: file,
				Message: fmt.Sprintf("does not match its manifest: %s", out.describe()),
				Fix:     "update the lockfile with the package manager, such as " + lf.fix + ", and commit it"})
		}
	}
	return Pass()
}

// joinRel joins a component path and a file name, slash separated.
func joinRel(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}
