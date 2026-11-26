package contract_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// withContract is files with vibe.yaml declaring contract version v.
func withContract(t *testing.T, files map[string]string, v string) map[string]string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(probeGoFixture, "vibe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "contract: 1.2.0\n") {
		t.Fatal("the probe fixture's vibe.yaml does not declare contract: 1.2.0")
	}
	out := maps.Clone(files)
	out["vibe.yaml"] = strings.Replace(string(config), "contract: 1.2.0\n", "contract: "+v+"\n", 1)
	return out
}

// honouringGuard is a fixture guard that lets a call whose payload
// matches the shell case pattern through when policy.json holds want,
// and hands every other call to vibe-guard: a third-party guard that
// honours an exception its own way.
func honouringGuard(pattern, want string) string {
	return `input=$(cat); case "$input" in ` + pattern + `) grep -qF '` + want + `' .claude/hooks/policy.json && exit 0;; esac; ` +
		`printf '%s' "$input" | vibe-guard hook`
}

// How A-02 and A-09 read an exception under each contract (spec 0072,
// "The audit's reading under 2.0.0"): under 2.0.0 a command exception
// counts only when it prefixes a simple command the case's rule refuses,
// and no exception counts for a fixed case; under 1.2.0 the reading is
// as before.
func TestGuardProbeExceptionReadingByContract(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	requireBash(t)
	bin := builtVibeGuard(t)
	settings := ".claude/settings.json"
	policy := ".claude/hooks/policy.json"
	exception := func(rule, match string) string {
		return `{"version": 2, "exceptions": [{"rule": "` + rule + `", "match": "` + match + `", "reason": "test"}]}` + "\n"
	}
	uses := func(rule, match string, cases ...string) []contract.GuardExceptionUse {
		var out []contract.GuardExceptionUse
		for _, c := range cases {
			out = append(out, contract.GuardExceptionUse{Rule: rule, Match: match, Case: c})
		}
		return out
	}

	// "cd /tmp && rm -rf": the rule refuses rm -rf, not cd /tmp. The hook
	// lets the case through while policy.json holds the exception.
	compound := map[string]string{
		settings: guardSettings(honouringGuard(`*'"cd /tmp '*' rm -rf"'*`, `"cd /tmp"`)),
		policy:   exception("rm-rf", "cd /tmp"),
	}
	// The Write calls of the unlock marker by a relative path, which the
	// hook lets through while policy.json holds the exception.
	markerCases := []string{
		"A-08 (#106): Write the marker, expired unlock",
		"A-08 (#106): Write the marker, no unlock",
		"A-08 (#106): Write the marker, valid unlock",
	}
	fixed := map[string]string{
		settings: guardSettings(honouringGuard(`*'"file_path":".git/vibe-unlock"'*'"tool_name":"Write"'*`, `.git/vibe-unlock`)),
		policy:   exception("guardrail-settings", ".git/vibe-unlock"),
	}

	t.Run("an exception for another simple command of a compound case counts under 1.2.0", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, compound, contract.Contract1)), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
		if want := uses("rm-rf", "cd /tmp", "rm -rf at end of command"); !slices.Equal(rep.GuardExceptionUses, want) {
			t.Errorf("GuardExceptionUses = %+v, want %+v", rep.GuardExceptionUses, want)
		}
	})
	t.Run("an exception for another simple command of a compound case does not count under 2.0.0", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, compound, contract.Contract2)), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail,
			`corpus case "rm -rf at end of command" (Bash "cd /tmp && rm -rf") should be refused (guard rule rm-rf)`)
		if got := casesNamed(t, rep); !slices.Equal(got, []string{"rm -rf at end of command"}) {
			t.Errorf("A-02 names %q, want only rm -rf at end of command", got)
		}
		if len(rep.GuardExceptionUses) != 0 {
			t.Errorf("GuardExceptionUses = %+v, want none", rep.GuardExceptionUses)
		}
	})
	t.Run("an exception for a fixed case counts under 1.2.0", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, fixed, contract.Contract1)), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultPass)
		var got []string
		for _, u := range rep.GuardExceptionUses {
			got = append(got, u.Case)
		}
		slices.Sort(got)
		if !slices.Equal(got, markerCases) {
			t.Errorf("GuardExceptionUses cases = %q, want %q", got, markerCases)
		}
	})
	t.Run("no exception counts for a fixed case under 2.0.0", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, guardFixture(t, withContract(t, fixed, contract.Contract2)), []string{"A-02"}, bin)
		assertPart(t, rep, "A-02", contract.Probe, contract.ResultFail,
			`corpus case "A-08 (#106): Write the marker, no unlock" (Write ".git/vibe-unlock") should be refused (guard rule guardrail-settings)`)
		if got := casesNamed(t, rep); !slices.Equal(got, markerCases) {
			t.Errorf("A-02 names %q, want %q", got, markerCases)
		}
		if len(rep.GuardExceptionUses) != 0 {
			t.Errorf("GuardExceptionUses = %+v, want none", rep.GuardExceptionUses)
		}
	})
	t.Run("A-09 counts an exception for the command its rule refuses under 2.0.0", func(t *testing.T) {
		t.Parallel()
		// The A-09 cases are single simple commands and none is fixed, so
		// the two readings agree on them; A-09 reads through A-02's code.
		rep := audit(t, guardFixture(t, withContract(t, map[string]string{
			settings: guardSettings(referenceGuard),
			policy:   exception("git-skip-hooks", "git commit --no-verify -m wip"),
		}, contract.Contract2)), []string{"A-09"}, bin)
		assertPart(t, rep, "A-09", contract.Probe, contract.ResultPass)
		want := uses("git-skip-hooks", "git commit --no-verify -m wip",
			"A-09: git commit --no-verify", "A-09: git commit --no-verify in PowerShell")
		if !slices.Equal(rep.GuardExceptionUses, want) {
			t.Errorf("GuardExceptionUses = %+v, want %+v", rep.GuardExceptionUses, want)
		}
	})
}
