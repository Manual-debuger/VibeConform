package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGoldens rewrites the text report goldens from the current audit:
//
//	go test ./internal/cli -run TestContractReportGolden -update
//
// Review the diff before committing it. Each golden changes whenever a
// rule's check or a finding's wording changes.
var updateGoldens = flag.Bool("update", false, "rewrite the contract report goldens; review the diff, then commit it")

// The text report goldens (spec 0052, "Testing Decisions"): the
// committed files replace the contract draft's output example. They are
// LF: .gitattributes gives testdata eol=lf, and the comparison is byte
// for byte, so a golden checked out with CRLF fails instead of being
// normalized silently.
func TestContractReportGolden(t *testing.T) {
	cases := []struct {
		name  string
		edits []fixtureEdit
		args  []string
	}{
		{name: "baseline"},
		{name: "static", args: []string{"--static"}},
		{name: "mixed", edits: []fixtureEdit{
			// V-04 fails (MUST); R-02 fails (SHOULD) with an expired
			// waiver; R-03 is waived; C-05 is not applicable, so its
			// waiver is unused.
			withFile("vibe.yaml", configWith(
				section{"platforms", "platforms: [linux]\n"},
				section{"entrypoints", "entrypoints:\n  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n"},
				section{"waivers", "waivers:\n" +
					"  - rule: R-02\n    reason: rewriting AGENTS.md\n    until: 2000-01-01\n" +
					"  - rule: R-03\n    reason: templates move to the org repository\n    until: 2999-12-31\n" +
					"  - rule: C-05\n    reason: one platform for now\n    until: 2999-12-31\n"},
			)),
			withFile("AGENTS.md", "Run the checks before you finish.\n"),
			withoutFile(".github/pull_request_template.md"),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runVibeAudit(t, append([]string{"--repo-root", contractFixture(t, tc.edits...)}, tc.args...)...)
			assertExit(t, r, 1)
			golden := filepath.Join("testdata", "contract", "report", tc.name+".golden")
			if *updateGoldens {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(r.stdout), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v; run with -update to create it", err)
			}
			if bytes.Contains(want, []byte("\r")) {
				t.Fatalf("%s has CRLF line endings; it must be checked out LF (.gitattributes)", golden)
			}
			if r.stdout != string(want) {
				t.Errorf("report differs from %s; review the change, then run with -update\n--- got\n%s\n--- want\n%s",
					golden, r.stdout, want)
			}
		})
	}
}
