package probetest_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// V-07 (spec 0061, #151): verify catches a secret planted once per
// repository, in the first component in configuration order that has
// tests. The fixture's "ci secrets" step is its secret scanner.

// withSecretScan is a verify that runs the fixture's secret scanner
// beside V-01's steps.
var withSecretScan = withEntrypoints("  verify: [go run ./ci fmt-check, go run ./ci typecheck, go run ./ci test, go run ./ci secrets]\n")

// threeComponents declares three Go components in an order that differs
// from path order: lib has no tests, and more and calc both have.
func threeComponents(entrypoints string) []fixtureEdit {
	return []fixtureEdit{
		withFile("more/more.go", "// Package more is a second component with tests.\npackage more\n\n// One returns one.\nfunc One() int { return 1 }\n"),
		withFile("more/more_test.go", "package more\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {\n\tif One() != 1 {\n\t\tt.Fatal(\"wrong\")\n\t}\n}\n"),
		probeConfig(
			section{"components", "components:\n" +
				"  - path: lib\n    language: go\n" +
				"  - path: more\n    language: go\n" +
				"  - path: calc\n    language: go\n"},
			section{"entrypoints", probeEntrypoints(entrypoints)}),
	}
}

// secretRuns audits V-07 alone and returns the report and the label of
// every entrypoint run.
func secretRuns(t *testing.T, dir string) (*contract.Report, []string) {
	t.Helper()
	var progress bytes.Buffer
	rep := audit(t, dir, ids("V-07"), contract.ProbeOptions{Progress: &progress})
	return rep, strings.Split(strings.TrimSpace(progress.String()), "\n")
}

// planted counts the runs with the secret defect planted.
func planted(runs []string) int {
	n := 0
	for _, r := range runs {
		if strings.Contains(r, "with the secret defect") {
			n++
		}
	}
	return n
}

func TestRuleV07(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips this test, which runs the contract probes; task verify runs it")
	}
	t.Parallel()
	t.Run("a verify with a secret scanner passes", func(t *testing.T) {
		t.Parallel()
		rep, runs := secretRuns(t, probeFixture(t, withSecretScan))
		assertPart(t, rep, "V-07", contract.Probe, pass)
		if planted(runs) != 1 {
			t.Errorf("runs with the secret planted = %d, want 1: %q", planted(runs), runs)
		}
	})
	t.Run("a verify without one fails and names the component", func(t *testing.T) {
		t.Parallel()
		rep, _ := secretRuns(t, probeFixture(t))
		assertPart(t, rep, "V-07", contract.Probe, fail, "vibe.yaml", "component .", missed("secret"),
			"calc/vibe_probe_secret.txt", "a secret scanner")
	})
	t.Run("several components run verify once, in the first with tests in configuration order", func(t *testing.T) {
		t.Parallel()
		rep, runs := secretRuns(t, probeFixture(t, threeComponents("")...))
		assertPart(t, rep, "V-07", contract.Probe, fail, "component more", "more/vibe_probe_secret.txt")
		if planted(runs) != 1 {
			t.Errorf("runs with the secret planted = %d, want 1: %q", planted(runs), runs)
		}
		for _, r := range runs {
			if strings.Contains(r, "verify-fast") {
				t.Errorf("V-07 ran verify-fast: %q", r)
			}
		}
	})
	t.Run("a repository without tests leaves it undecided", func(t *testing.T) {
		t.Parallel()
		rep, _ := secretRuns(t, probeFixture(t, withSecretScan, withoutFile("calc/calc_test.go")))
		assertPart(t, rep, "V-07", contract.Probe, undecided, "component . has no tests")
	})
}
