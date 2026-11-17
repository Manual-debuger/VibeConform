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
func checkV01Execution(env *Env, _ Needed) PartResult {
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
func checkV01Probe(env *Env, _ Needed) PartResult {
	return defectProbe{rule: "V-01", entrypoint: EntrypointVerify, noTestsFail: true}.run(env)
}

// checkV02Probe: with each planted defect in each component, verify-fast
// exits non-zero.
func checkV02Probe(env *Env, _ Needed) PartResult {
	return defectProbe{rule: "V-02", entrypoint: EntrypointVerifyFast}.run(env)
}

// checkV05Probe: with the type-error defect planted in each Python
// component, verify exits non-zero (spec 0061). Other components are
// skipped: their type errors are compile errors, which V-01 shows.
func checkV05Probe(env *Env, _ Needed) PartResult {
	return defectProbe{rule: "V-05", entrypoint: EntrypointVerify,
		components: func(env *Env) []Component { return pythonComponents(env.Config) }}.run(env)
}

// pythonComponents lists the Python components in configuration order.
func pythonComponents(cfg *Config) []Component {
	var out []Component
	for _, c := range cfg.Components {
		if c.Language == LanguagePython {
			out = append(out, c)
		}
	}
	return out
}

// hasPythonComponent is V-05's applicability.
func hasPythonComponent(cfg *Config) (bool, string) {
	if len(pythonComponents(cfg)) > 0 {
		return true, ""
	}
	return false, "no component is Python"
}

// checkV06Probe: with the complexity defect in each component, verify
// exits non-zero (spec 0061). A component without tests leaves it
// undecided.
func checkV06Probe(env *Env, _ Needed) PartResult {
	return defectProbe{rule: "V-06", entrypoint: EntrypointVerify}.run(env)
}

// checkV07Probe: with the secret defect planted once per repository,
// verify exits non-zero (spec 0061). A secret does not depend on the
// language, so one component is enough: the first in configuration order
// that has tests.
func checkV07Probe(env *Env, _ Needed) PartResult {
	return defectProbe{rule: "V-07", entrypoint: EntrypointVerify, firstWithTests: true}.run(env)
}

// checkV08Probe: with V-01's static-analysis defect under the language's
// blanket suppressions planted in each component, verify exits non-zero.
// A component without tests leaves it undecided (spec 0061).
func checkV08Probe(env *Env, _ Needed) PartResult {
	return defectProbe{rule: "V-08", entrypoint: EntrypointVerify}.run(env)
}

// defectProbe is one rule's planted-defect probe: in which components it
// plants the rule's defect kinds (ruleDefectKinds), and which entrypoint
// must exit non-zero with each. A rule with its own defect kinds lists
// them in ruleDefectKinds; each kind needs a sample in the language
// record of every language it is planted in, and a catcher in defectCatchers.
type defectProbe struct {
	// rule is the ID of the rule the probe decides; its kinds are
	// ruleDefectKinds[rule], and the Fix names it.
	rule       string
	entrypoint string
	// noTestsFail says whether a component with no tests fails the part
	// (V-01) or leaves it undecided.
	noTestsFail bool
	// components picks the components to plant in, in configuration
	// order. nil plants in every component.
	components func(env *Env) []Component
	// firstWithTests plants in the first picked component that has tests
	// only. When none has, the first component is reported as having no
	// tests.
	firstWithTests bool
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
			if !p.firstWithTests {
				results = append(results, noTests(env, comp, p.noTestsFail))
			}
			continue
		}
		if baseline == nil {
			b := baselinePasses(env, p.entrypoint)
			baseline = &b
		}
		if baseline.Result != ResultPass {
			results = append(results, *baseline)
		} else {
			results = append(results, p.component(env, comp))
		}
		if p.firstWithTests {
			break
		}
	}
	if len(results) == 0 {
		// firstWithTests, and no component has tests.
		results = append(results, noTests(env, comps[0], p.noTestsFail))
	}
	return mergeRuns(dedupe(results)...)
}

// noTests is the result for a component with nowhere to plant.
func noTests(env *Env, comp Component, fail bool) PartResult {
	msg := errNoTests{comp.Path}.Error()
	if fail {
		return Fail(Finding{File: env.Config.File, Message: msg,
			Fix: "add a test file to component " + comp.Path + " (" + comp.language().testFilePattern() + "), so that verify can be shown to run its tests"})
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
	var missedKinds []defectKind
	var planted []string
	var undecided []PartResult
	for _, kind := range ruleDefectKinds[p.rule] {
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
			missedKinds = append(missedKinds, kind)
			planted = append(planted, d.Path)
		}
	}
	res := Pass()
	if len(missedKinds) > 0 {
		res = Fail(Finding{File: env.Config.File,
			Message: fmt.Sprintf("%s (%s) exits 0 in component %s with a planted defect (%s); the defect kinds that did not make it fail: %s",
				name, strings.Join(env.EntrypointSource(name), "; "), comp.Path, strings.Join(planted, ", "), joinKinds(missedKinds)),
			Fix: fmt.Sprintf("make entrypoints.%s run %s; vibe audit --explain %s shows the planted files",
				name, kindCatchers(missedKinds), p.rule)})
	}
	return mergeRuns(append([]PartResult{res}, undecided...)...)
}

// joinKinds lists defect kinds, comma separated.
func joinKinds(kinds []defectKind) string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}

// kindCatchers names what an entrypoint must run to catch defect kinds.
func kindCatchers(kinds []defectKind) string {
	var tools []string
	for _, k := range kinds {
		if c, ok := defectCatchers[k]; ok {
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
func checkA05Probe(env *Env, _ Needed) PartResult {
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
