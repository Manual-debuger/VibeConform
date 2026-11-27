package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// updateGoldens rewrites the text report goldens from the current audit:
//
//	go test ./internal/cli -run TestContractReportGolden -update
//
// Review the diff before committing it. Each golden changes whenever a
// rule's check or a finding's wording changes.
var updateGoldens = flag.Bool("update", false, "rewrite the contract report goldens; review the diff, then commit it")

// goldenContract is the contract the golden reports' fixtures declare:
// the 2.x contract this vibe implements (spec 0074).
var goldenContract = contracttest.Section{Key: "contract", Body: "contract: 2.1.0\n"}

// unguardedSettings is the baseline's .claude/settings.json without its
// PreToolUse hook: no guard, and the other hooks as they are.
const unguardedSettings = `{
  "hooks": {
    "SessionStart": [
      {"matcher": "startup|resume|clear", "hooks": [{"type": "command", "command": "task -x hook:context"}]}
    ],
    "PostToolUse": [
      {"matcher": "Write|Edit|MultiEdit", "hooks": [{"type": "command", "command": "task fmt"}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "task verify:fast 1>&2 || exit 2"}]}
    ]
  }
}
`

func requiredGoldenEdits() []contracttest.Edit {
	return []contracttest.Edit{
		contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(goldenContract, contracttest.Section{Key: "require", Body: "require: [V-05, V-06, R-03]\n"})),
		contracttest.WithoutFile(".github/pull_request_template.md"),
	}
}

// The text report goldens (spec 0052, "Testing Decisions"): the
// committed files replace the contract draft's output example. They are
// LF: .gitattributes gives testdata eol=lf, and the comparison is byte
// for byte, so a golden checked out with CRLF fails instead of being
// normalized silently.
func TestContractReportGolden(t *testing.T) { //nolint:gocognit // baseline: pre-existing complexity
	t.Parallel()
	cases := []struct {
		name  string
		edits []contracttest.Edit
		args  []string
	}{
		{name: "baseline", edits: []contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(goldenContract))}},
		{name: "static", edits: []contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(goldenContract))}, args: []string{"--static"}},
		{name: "mixed", edits: []contracttest.Edit{
			// V-04 fails (MUST); R-02 fails (SHOULD) with an expired
			// waiver; R-03 is waived; V-05 is not applicable (no Python
			// component), so its waiver is unused. Under 2.1.0, C-05
			// follows platforms: one Linux runner is enough (spec 0074).
			contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(goldenContract,
				contracttest.Section{Key: "platforms", Body: "platforms: [linux]\n"},
				contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  verify: task verify\n  verify-fast: task verify:fast\n  fmt-check: task fmt:check\n"},
				contracttest.Section{Key: "waivers", Body: "waivers:\n" +
					"  - rule: R-02\n    reason: rewriting AGENTS.md\n    until: 2000-01-01\n" +
					"  - rule: R-03\n    reason: templates move to the org repository\n    until: 2999-12-31\n" +
					"  - rule: V-05\n    reason: no type checker for Python yet\n    until: 2999-12-31\n"},
			)),
			contracttest.WithFile("AGENTS.md", "Run the checks before you finish.\n"),
			contracttest.WithoutFile(".github/pull_request_template.md"),
		}},
		// Required SHOULD rules (spec 0063) are marked wherever the report
		// lists them: R-03 fails, V-05 is not applicable, V-06 cannot be
		// decided or, with --static, does not run.
		{name: "required", edits: requiredGoldenEdits()},
		{name: "required-static", edits: requiredGoldenEdits(), args: []string{"--static"}},
		// The guard set (spec 0074): SHOULD under 2.1.0, and marked
		// required wherever the report lists it when vibe.yaml requires
		// it. Under 1.2.0 it is MUST.
		{name: "guard-set-required", edits: []contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(goldenContract,
			contracttest.Section{Key: "require", Body: "require: [A-01, A-02, A-03, A-04, A-08]\n"}))}},
		{name: "contract-1", edits: []contracttest.Edit{contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(
			contracttest.Section{Key: "contract", Body: "contract: 1.2.0\n"}))}},
		// No guard and the guard set not required (spec 0074): A-01
		// fails, and the rules that test the guard are not applicable.
		{name: "no-guard", edits: []contracttest.Edit{
			contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(goldenContract)),
			contracttest.WithFile(".claude/settings.json", unguardedSettings),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runVibeAudit(t, append([]string{"--repo-root", contracttest.GitFixture(t, contracttest.Baseline, tc.edits...)}, tc.args...)...)
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
