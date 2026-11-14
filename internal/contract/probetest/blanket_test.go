package probetest_test

import (
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// V-08 (spec 0061, "V-08, blanket suppressions"): verify still catches
// V-01's static-analysis defect under a blanket //nolint. The Go probe
// fixture's vet-nolint step is a linter that honors //nolint, as
// golangci-lint does, and its nolintlint step refuses a //nolint that
// names no linter.
func TestRuleV08BlanketSuppression(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	t.Run("a verify that refuses blanket suppressions passes, and verify-fast is not run", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints(
			"  verify: [go run ./ci fmt-check, go run ./ci vet-nolint, go run ./ci nolintlint, go run ./ci test]\n"+
				"  verify-fast: go run ./ci fail\n")), ids("V-08"), inherit)
		assertPart(t, rep, "V-08", contract.Probe, pass)
	})
	t.Run("a verify whose linter honors a blanket //nolint fails and names the component", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints(
			"  verify: [go run ./ci fmt-check, go run ./ci vet-nolint, go run ./ci test]\n")), ids("V-08"), inherit)
		assertPart(t, rep, "V-08", contract.Probe, fail, "vibe.yaml", "component .",
			missed("blanket suppression"), "calc/vibe_probe_blanket_suppression_test.go", "--explain V-08")
	})
	t.Run("the same verify without the blanket suppression catches V-01's defect", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withEntrypoints(
			"  verify: [go run ./ci fmt-check, go run ./ci vet-nolint, go run ./ci test]\n")), ids("V-01"), inherit)
		assertPart(t, rep, "V-01", contract.Probe, pass)
	})
	t.Run("a component without tests leaves V-08 undecided", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t, withoutFile("calc/calc_test.go")), ids("V-08"), inherit)
		assertPart(t, rep, "V-08", contract.Probe, undecided, "component . has no tests")
	})
}
