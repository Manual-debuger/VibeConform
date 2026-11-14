package probetest_test

import (
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// V-06 (spec 0061): verify catches a function of cyclomatic complexity
// 41 and cognitive complexity 40, planted as a non-test file in every
// component. The fixture's ci program has a complexity step; verify
// without it misses the sample.
func TestRuleV06(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	t.Run("a verify with a complexity limit passes, and verify-fast is not run", func(t *testing.T) {
		t.Parallel()
		// verify-fast fails on purpose: a V-06 that ran it would be
		// undecided.
		rep := audit(t, probeFixture(t, withEntrypoints(
			"  verify: [go run ./ci fmt-check, go run ./ci typecheck, go run ./ci test, go run ./ci complexity]\n"+
				"  verify-fast: go run ./ci fail\n")), ids("V-06"), inherit)
		assertPart(t, rep, "V-06", contract.Probe, pass)
	})
	t.Run("a verify without a complexity limit fails and names the component", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t), ids("V-06"), inherit)
		assertPart(t, rep, "V-06", contract.Probe, fail, "vibe.yaml", "component .", missed("complexity"),
			"calc/vibe_probe_complexity.go", "a complexity limit of 40 or less")
	})
	t.Run("a component without tests leaves V-06 undecided", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withoutFile("calc/calc_test.go")), ids("V-06"), inherit)
		assertPart(t, rep, "V-06", contract.Probe, undecided, "component . has no tests")
	})
}
