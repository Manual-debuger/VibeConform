package contract

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The planted-defect probes of #104 (spec 0053, "The rules"): V-01,
// V-02 and A-05's probe part. Each plants one defect at a time in a reset
// probe copy.

// checkV01Execution: the baseline verify exits 0 in the reset copy.
func checkV01Execution(env *Env) PartResult {
	if f := needEntrypoint(env, EntrypointVerify); f != nil {
		return Fail(*f)
	}
	run, err := env.probe.Baseline(env, EntrypointVerify)
	switch {
	case err != nil:
		return cannotReset(err)
	case run.Missing != nil:
		return Fail(*run.Missing)
	case !run.Ran():
		return undecidedRun(run.runOutcome)
	case run.ExitCode != 0:
		return Fail(Finding{File: env.Config.File,
			Message: "in the probe copy, " + run.describe(),
			Fix:     "make entrypoints.verify pass on a clean checkout of the repository"})
	}
	return Pass()
}

// checkV01Probe: with each planted defect in each component, verify
// exits non-zero. A component without tests fails.
func checkV01Probe(env *Env) PartResult {
	return defectProbe{rule: "V-01", entrypoint: EntrypointVerify, kinds: v01Kinds, noTestsFail: true}.run(env)
}

// checkV02Probe: with each planted defect in each component, verify-fast
// exits non-zero.
func checkV02Probe(env *Env) PartResult {
	return defectProbe{rule: "V-02", entrypoint: EntrypointVerifyFast, kinds: v01Kinds}.run(env)
}

// checkV06Probe: with the complexity defect in each component, verify
// exits non-zero (spec 0061). A component without tests leaves it
// undecided.
func checkV06Probe(env *Env) PartResult {
	return defectProbe{rule: "V-06", entrypoint: EntrypointVerify, kinds: []defectKind{defectComplexity}}.run(env)
}

// defectProbe is one rule's planted-defect probe: which defect kinds it
// plants, in which components, and which entrypoint must exit non-zero
// with each. A rule with its own defect kinds builds one with them; each
// kind needs a sample in defectSamples, for every language it is planted
// in, and a catcher in defectCatchers.
type defectProbe struct {
	// rule is the ID of the rule the probe decides; the Fix names it.
	rule       string
	entrypoint string
	kinds      []defectKind
	// noTestsFail says whether a component with no tests fails the part
	// (V-01) or leaves it undecided.
	noTestsFail bool
	// components picks the components to plant in, in configuration
	// order. nil plants in every component.
	components func(env *Env) []Component
}

// run plants each defect kind in each picked component, one at a time
// in a reset probe copy, and runs the entrypoint with each. Its baseline
// must pass first: otherwise a defect that "made it fail" would prove
// nothing. The baseline runs once, and only when some component has
// tests.
func (p defectProbe) run(env *Env) PartResult {
	if f := needEntrypoint(env, p.entrypoint); f != nil {
		return Fail(*f)
	}
	comps := env.Config.Components
	if p.components != nil {
		comps = p.components(env)
	}
	if len(comps) == 0 {
		return Undecided(Finding{Message: "cannot decide: no component to plant a defect in"})
	}
	var results []PartResult
	var baseline *PartResult
	for _, comp := range comps {
		if _, _, ok := plantDir(env, comp); !ok {
			results = append(results, noTests(env, comp, p.noTestsFail))
			continue
		}
		if baseline == nil {
			b := baselinePasses(env, p.entrypoint)
			baseline = &b
		}
		if baseline.Result != ResultPass {
			results = append(results, *baseline)
			continue
		}
		results = append(results, p.component(env, comp))
	}
	return mergeRuns(dedupe(results)...)
}

// noTests is the result for a component with nowhere to plant.
func noTests(env *Env, comp Component, fail bool) PartResult {
	msg := errNoTests{comp.Path}.Error()
	if fail {
		return Fail(Finding{File: env.Config.File, Message: msg,
			Fix: "add a test file to component " + comp.Path + " (" + testFilePattern(comp.Language) + "), so that verify can be shown to run its tests"})
	}
	return Undecided(Finding{Message: "cannot decide: " + msg + ", so there is nowhere to plant a defect"})
}

// baselinePasses is the precondition of a defect probe: the entrypoint
// exits 0 in the reset copy.
func baselinePasses(env *Env, name string) PartResult {
	run, err := env.probe.Baseline(env, name)
	switch {
	case err != nil:
		return cannotReset(err)
	case run.Missing != nil:
		return Fail(*run.Missing)
	case !run.Ran():
		return undecidedRun(run.runOutcome)
	case run.ExitCode != 0:
		return Undecided(Finding{File: env.Config.File,
			Message: fmt.Sprintf("cannot decide: %s fails without a planted defect, so a defect that makes it fail proves nothing: %s", name, run.describe()),
			Fix:     "make entrypoints." + name + " pass on a clean checkout of the repository"})
	}
	return Pass()
}

// component plants each defect kind in one component and runs the
// entrypoint with it. Its finding names the entrypoint, the component,
// the planted files and the kinds that did not make the entrypoint fail.
func (p defectProbe) component(env *Env, comp Component) PartResult {
	s, name := env.probe, p.entrypoint
	var missedKinds, planted []string
	var undecided []PartResult
	for _, kind := range p.kinds {
		if err := s.Reset(); err != nil {
			return cannotReset(err)
		}
		d, err := defectFor(env, comp, kind)
		if err != nil {
			undecided = append(undecided, Undecided(Finding{Message: fmt.Sprintf("cannot decide: nowhere to plant the %s defect: %s", kind, err)}))
			continue
		}
		if err := s.Plant(d.Path, d.Content); err != nil {
			undecided = append(undecided, Undecided(Finding{Message: "cannot decide: cannot plant " + d.Path + ": " + err.Error()}))
			continue
		}
		run := s.RunEntrypoint(env, name, fmt.Sprintf("%s with the %s defect in %s", name, kind, comp.Path))
		switch {
		case run.Missing != nil:
			return Fail(*run.Missing)
		case !run.Ran():
			undecided = append(undecided, undecidedRun(run.runOutcome))
		case run.ExitCode == 0:
			missedKinds = append(missedKinds, string(kind))
			planted = append(planted, d.Path)
		}
	}
	res := Pass()
	if len(missedKinds) > 0 {
		res = Fail(Finding{File: env.Config.File,
			Message: fmt.Sprintf("%s (%s) exits 0 in component %s with a planted defect (%s); the defect kinds that did not make it fail: %s",
				name, strings.Join(env.EntrypointSource(name), "; "), comp.Path, strings.Join(planted, ", "), strings.Join(missedKinds, ", ")),
			Fix: fmt.Sprintf("make entrypoints.%s run %s; vibe audit --explain %s shows the planted files",
				name, kindCatchers(missedKinds), p.rule)})
	}
	return mergeRuns(append([]PartResult{res}, undecided...)...)
}

// kindCatchers names what an entrypoint must run to catch defect kinds.
func kindCatchers(kinds []string) string {
	var tools []string
	for _, k := range kinds {
		if c, ok := defectCatchers[defectKind(k)]; ok {
			tools = append(tools, c)
		}
	}
	return strings.Join(tools, " and ")
}

func cannotReset(err error) PartResult {
	return Undecided(Finding{Message: "cannot decide: cannot reset the probe copy: " + err.Error()})
}

// dedupe drops repeated identical findings, such as one baseline failure
// reported for every component.
func dedupe(results []PartResult) []PartResult {
	seen := map[string]bool{}
	var out []PartResult
	for _, r := range results {
		var kept []Finding
		for _, f := range r.Findings {
			key := f.File + "\x00" + f.Message
			if !seen[key] {
				seen[key] = true
				kept = append(kept, f)
			}
		}
		r.Findings = kept
		out = append(out, r)
	}
	return out
}

// checkA05Probe: with the failing-test defect of the first component
// that has tests planted, some Stop hook exits 2 for stop_hook_active
// false, and every Stop hook exits 0 for stop_hook_active true.
func checkA05Probe(env *Env) PartResult {
	fix := "add a Stop hook that runs " + quoteCommands(env.EntrypointSource(EntrypointVerifyFast)) +
		" and exits 2 when it fails, unless stop_hook_active is true"
	settings, res, ok := env.readClaudeSettings(fix)
	if !ok {
		return res
	}
	hooks := settings.commandHooks(eventStop)
	if len(hooks) == 0 {
		return Fail(Finding{File: claudeSettingsFile, Message: "has no Stop hook, so nothing blocks the end of a turn with a failing test", Fix: fix})
	}
	d, comp, res, ok := a05Defect(env)
	if !ok {
		return res
	}
	s := env.probe
	if ok, why := s.HasBash(); !ok {
		return Undecided(Finding{Message: "cannot decide: " + why})
	}

	run := func(h settingsHook, active bool) runOutcome {
		if err := s.Reset(); err != nil {
			return runOutcome{Label: "the Stop hook", Undecided: "cannot reset the probe copy: " + err.Error()}
		}
		if err := s.Plant(d.Path, d.Content); err != nil {
			return runOutcome{Label: "the Stop hook", Undecided: "cannot plant " + d.Path + ": " + err.Error()}
		}
		payload := s.HookPayload(eventStop, map[string]any{"stop_hook_active": active})
		label := fmt.Sprintf("the Stop hook with the test defect in %s, stop_hook_active %t", comp.Path, active)
		return s.RunHook(label, h.command, h.limit, payload, RunOpts{})
	}

	// Some hook blocks the turn while it is not yet continued.
	var blocked bool
	var notBlocked []string
	var undecided []PartResult
	for _, h := range hooks {
		out := run(h, false)
		switch {
		case !out.Ran():
			undecided = append(undecided, undecidedRun(out))
		case out.ExitCode == 2:
			blocked = true
		default:
			notBlocked = append(notBlocked, out.describe())
		}
	}
	var results []PartResult
	switch {
	case blocked:
	case len(undecided) > 0:
		results = append(results, undecided...)
	default:
		results = append(results, Fail(Finding{File: claudeSettingsFile,
			Message: fmt.Sprintf("with a failing test planted (%s) and stop_hook_active false, no Stop hook exited 2: %s", d.Path, strings.Join(notBlocked, "; ")),
			Fix:     fix}))
	}

	// Every hook lets a continued turn end, so the agent cannot loop.
	for _, h := range hooks {
		out := run(h, true)
		switch {
		case !out.Ran():
			results = append(results, undecidedRun(out))
		case out.ExitCode != 0:
			results = append(results, Fail(Finding{File: claudeSettingsFile,
				Message: fmt.Sprintf("with stop_hook_active true, %s", out.describe()),
				Fix:     "make the Stop hook exit 0 when its payload has stop_hook_active true, so that a continued turn can end"}))
		}
	}
	return mergeRuns(results...)
}

// a05Defect is the failing-test defect of the first component, by path,
// that has tests.
func a05Defect(env *Env) (plantedDefect, Component, PartResult, bool) {
	comps := slices.Clone(env.Config.Components)
	slices.SortStableFunc(comps, func(a, b Component) int { return cmp.Compare(a.Path, b.Path) })
	for _, comp := range comps {
		d, err := defectFor(env, comp, defectTest)
		var nt errNoTests
		switch {
		case errors.As(err, &nt):
			continue
		case err != nil:
			return d, comp, Undecided(Finding{Message: "cannot decide: nowhere to plant the test defect: " + err.Error()}), false
		}
		return d, comp, PartResult{}, true
	}
	return plantedDefect{}, Component{}, Undecided(Finding{Message: "cannot decide: no component has tests, so there is nowhere to plant the test defect"}), false
}
