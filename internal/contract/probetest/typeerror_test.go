package probetest_test

import (
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// V-05 (spec 0061): with the type-error defect planted in each Python
// component, verify exits non-zero. The fixture is the Go probe fixture
// with a Python component beside its Go one; its ci program's
// pytypecheck step stands in for a type checker, so that verify catches
// or misses the defect on purpose.

// typeErrorFixture is the Go probe fixture with components calc (Go)
// and py (Python), verify as given, and edits.
func typeErrorFixture(t *testing.T, verify string, edits ...fixtureEdit) string {
	t.Helper()
	cfg := probeConfig(
		section{"components", "components:\n  - path: calc\n    language: go\n  - path: py\n    language: python\n"},
		section{"entrypoints", probeEntrypoints(verify)},
	)
	base := []fixtureEdit{
		withFile("py/app.py", "def answer() -> int:\n    return 42\n"),
		withFile("py/test_app.py", "def test_answer() -> None:\n    assert True\n"),
		cfg,
	}
	return probeFixture(t, append(base, edits...)...)
}

func TestRuleV05(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	const withTypeCheck = "  verify: [go run ./ci fmt-check, go run ./ci typecheck, go run ./ci pytypecheck, go run ./ci test]\n"
	t.Run("a verify that runs a type checker passes, and the Go component is skipped", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, typeErrorFixture(t, withTypeCheck), ids("V-05"), inherit)
		assertPart(t, rep, "V-05", contract.Probe, pass)
	})
	t.Run("a verify without a type checker fails and names the component", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, typeErrorFixture(t, ""), ids("V-05"), inherit)
		assertPart(t, rep, "V-05", contract.Probe, fail,
			"vibe.yaml", "component py", missed("type error"), "py/vibe_probe_type_error.py", "a type checker", "V-05")
	})
	t.Run("a verify that fails without a defect leaves V-05 undecided", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, typeErrorFixture(t, "  verify: go run ./ci fail\n"), ids("V-05"), inherit)
		assertPart(t, rep, "V-05", contract.Probe, undecided, "fails without a planted defect")
	})
	t.Run("a Python component without tests leaves V-05 undecided", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, typeErrorFixture(t, withTypeCheck, withoutFile("py/test_app.py")), ids("V-05"), inherit)
		assertPart(t, rep, "V-05", contract.Probe, undecided, "component py has no tests")
	})
	t.Run("a repository without a Python component makes V-05 not applicable", func(t *testing.T) {
		t.Parallel()
		rep := audit(t, probeFixture(t), ids("V-05"), inherit)
		rr := rule(t, rep, "V-05")
		if rr.Result != contract.ResultNotApplicable {
			t.Errorf("V-05 = %s, want not applicable\nfindings:\n%s", rr.Result, findingText(rr))
		}
	})
}
