package contract

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// The A-08 probe (#106; spec 0053, "A-08"): the corpus's A-08 cases run
// through the PreToolUse hooks in their unlock states, then every Stop
// hook runs with a valid marker in place and must leave no marker.
// Guard exceptions do not count: spec 0051's A-08 check says "refused".

// a08CasePrefix names the corpus's A-08 allow cases; its deny cases all
// have the guard rule ID guardrail-settings.
const a08CasePrefix = "A-08 ("

// a08Cases returns the corpus's A-08 cases, in corpus order.
func a08Cases() []GuardCase {
	var out []GuardCase
	for _, c := range guardCorpus() {
		if c.Rule == guardrule.GuardrailSettings || strings.HasPrefix(c.Name, a08CasePrefix) {
			out = append(out, c)
		}
	}
	return out
}

// unlockState names a case's unlock state for a finding.
func unlockState(state string) string {
	switch state {
	case UnlockValid:
		return "with a valid unlock marker"
	case UnlockExpired:
		return "with an expired unlock marker"
	}
	return "with no unlock marker"
}

// relockFix is the fix for a marker the Stop hooks leave behind.
const relockFix = "add a Stop hook that runs `vibe-guard relock`"

// checkA08 decides A-08's probe.
func checkA08(env *Env) PartResult {
	s := env.probe
	settings, res, ok := env.readClaudeSettings("add a PreToolUse hook that runs the guard")
	if !ok {
		return res
	}
	hooks, stops := settings.commandHooks(eventPreToolUse), settings.commandHooks(eventStop)
	if len(hooks)+len(stops) > 0 {
		if ok, why := s.HasBash(); !ok {
			return Undecided(Finding{File: claudeSettingsFile, Message: "cannot decide: " + why})
		}
	}
	defer func() { _ = s.setUnlock(UnlockNone) }()

	v := &verdicts{}
	a08Cases := a08Cases()
	runs, err := feedCases(s, hooks, a08Cases, "the PreToolUse hooks during A-08",
		func(c GuardCase) string { return "A-08 with " + describeGuardCase(c) }, RunOpts{})
	if err != nil {
		return Undecided(Finding{Message: "cannot decide: cannot write the unlock marker: " + err.Error()})
	}
	exceptions := policyExceptions(env)
	for _, run := range runs {
		c := run.c
		state := unlockState(c.Unlock)
		switch {
		case c.Want == WantDeny && run.refused():
		case run.undecided() != nil && (c.Want == WantDeny || !run.refused()):
			v.undecided(Finding{File: claudeSettingsFile,
				Message: "cannot decide: " + run.what() + " " + state + ": " + run.undecided().describe()})
		case c.Want == WantDeny:
			msg := fmt.Sprintf("%s should be refused %s; %s", run.what(), state, run.exits())
			for _, ex := range exceptionsFor(exceptions, c, s) {
				msg += fmt.Sprintf("; the exception %s %q in %s does not count: guard exceptions do not satisfy A-08",
					ex.Rule, ex.Match, guardPolicyFile)
			}
			v.fail(Finding{File: claudeSettingsFile, Message: msg,
				Fix: "make the guard refuse it (exit 2): only a valid unlock marker lets an agent change the " +
					"guardrail settings, and an agent never writes the marker or runs vibe unlock"})
		case run.refused():
			msg := fmt.Sprintf("%s should be allowed %s; %s", run.what(), state, run.exits())
			if line := run.refusal(); line != "" {
				msg += ": " + line
			}
			v.fail(Finding{File: claudeSettingsFile, Message: msg,
				Fix: "make the guard allow changes of the guardrail settings while the unlock marker in the common Git directory is valid"})
		}
	}
	a08Stop(s, stops, v)
	return v.result()
}

// a08Stop runs every Stop hook with a valid marker in place and a
// payload with stop_hook_active true; afterwards the marker must be gone.
// The copy is clean, so verify-fast passes and this is not a second A-05.
func a08Stop(s *probeSession, stops []settingsHook, v *verdicts) {
	if len(stops) == 0 {
		v.fail(Finding{File: claudeSettingsFile, Message: "no Stop hook removes the unlock marker", Fix: relockFix})
		return
	}
	if err := s.Reset(); err != nil {
		v.undecided(Finding{Message: "cannot decide: cannot reset the probe copy: " + err.Error()})
		return
	}
	defer func() { _ = s.Reset() }()
	if err := s.setUnlock(UnlockValid); err != nil {
		v.undecided(Finding{Message: "cannot decide: cannot write the unlock marker: " + err.Error()})
		return
	}
	payload := s.HookPayload(eventStop, map[string]any{"stop_hook_active": true})
	var exits []string
	var undecided *runOutcome
	for _, h := range stops {
		out := s.RunHook("A-08 Stop hook with a valid unlock marker", h.command, h.limit, payload, RunOpts{})
		if !out.Ran() {
			if undecided == nil {
				undecided = &out
			}
			exits = append(exits, fmt.Sprintf("hook %q: %s", shortCommand(h.command), out.Undecided))
			continue
		}
		exits = append(exits, fmt.Sprintf("hook %q exited %d", shortCommand(h.command), out.ExitCode))
	}
	_, err := os.Stat(filepath.Join(s.GitDir(), guardrule.UnlockMarkerFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		v.undecided(Finding{Message: "cannot decide: cannot read the unlock marker: " + err.Error()})
	case undecided != nil:
		v.undecided(Finding{File: claudeSettingsFile,
			Message: "cannot decide: a Stop hook did not end by itself: " + undecided.describe()})
	default:
		v.fail(Finding{File: claudeSettingsFile,
			Message: fmt.Sprintf("after the Stop hooks ran with a valid unlock marker, the marker %s still exists; %s",
				".git/"+guardrule.UnlockMarkerFile, strings.Join(exits, "; ")),
			Fix: relockFix + " before anything in it that can fail"})
	}
}
