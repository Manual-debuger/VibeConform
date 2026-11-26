package contract_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// updateGoldens rewrites the package's goldens from the current code:
// the explain goldens (go test ./internal/contract -run TestExplainGolden
// -update) and the CI findings goldens (ci_golden_test.go). It is the
// package's one -update flag; a second flag of that name would panic.
// Review the diff before committing it: a refactor such as spec 0066's
// or spec 0067's must leave every golden unchanged.
var updateGoldens = flag.Bool("update", false, "rewrite the goldens (explain and CI findings); review the diff, then commit it")

// explainText is what vibe audit --explain prints from the contract for
// one rule, beyond the facts every rule has: the rule's check, each
// part's check, and the rule's details (RuleDetails), such as the planted
// defect samples. --explain wraps the check; the golden keeps it whole.
func explainText(t *testing.T, id string) string {
	t.Helper()
	rule, ok := contract.LookupRule(id)
	if !ok {
		t.Fatalf("no rule %s", id)
	}
	var b strings.Builder
	b.WriteString("Check:\n" + rule.Check + "\n")
	for _, p := range rule.Parts {
		b.WriteString("\nPart check (" + string(p.Kind) + "):\n" + p.Check + "\n")
	}
	if details := contract.RuleDetails(id); details != "" {
		b.WriteString("\nDetails:\n" + details)
	}
	// V-07's sample is a PEM private-key block. The golden writes each
	// run of five dashes as pemDashes, so that it holds no literal
	// private-key block for secret scanners to report (.gitleaks.toml),
	// as the sample's own source does.
	return strings.ReplaceAll(b.String(), "-----", pemDashes)
}

// pemDashes stands for "-----" in the explain goldens.
const pemDashes = "{{five dashes}}"

// The explain goldens (spec 0066, "Testing Decisions"): D-01's check
// text, which names each lockfile's commands, and what --explain prints
// for V-01 to V-08, the planted defect samples among it. They pin the
// text byte for byte before the language knowledge moves into records.
// A-02 and R-01 pin what they say under each contract MAJOR (spec 0072).
// The goldens are LF (.gitattributes), so a CRLF checkout fails instead
// of being normalized silently.
func TestExplainGolden(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"D-01", "V-01", "V-02", "V-03", "V-04", "V-05", "V-06", "V-07", "V-08", "A-02", "R-01"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			assertGolden(t, filepath.Join("testdata", "explain", id+".golden"), explainText(t, id))
		})
	}
}

// assertGolden compares got with the golden file byte for byte, after
// rewriting the file with -update.
func assertGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *updateGoldens {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/contract -run TestExplainGolden -update to write it)", err)
	}
	if bytes.Contains(want, []byte("\r\n")) {
		t.Fatalf("%s has CRLF line endings; it must be checked out LF (.gitattributes)", golden)
	}
	if got != string(want) {
		t.Errorf("%s differs\ngot:\n%s\nwant:\n%s", golden, got, want)
	}
}
