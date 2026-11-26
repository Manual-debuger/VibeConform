package contract_test

import (
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// Under contract 2.0.0, R-01 and C-04 recognize a call of vibe by path:
// the last path element of a command's first word, after / or \, is the
// literal vibe or vibe.exe; the directory part may hold an expansion
// (spec 0072, "R-01 and C-04"). Under 1.2.0 the first word must be the
// literal vibe, as before. Each case runs under both contracts, through
// contract.Audit (ADR 0028).

// byPathCase is one input and the result it gives under each contract.
type byPathCase struct {
	name string
	// entrypoints, when set, replaces the lines under "entrypoints:".
	entrypoints string
	edits       []contracttest.Edit
	// v1 and v2 are the results under 1.2.0 and under 2.0.0; findings2
	// are substrings of the findings under 2.0.0.
	v1, v2    contract.Result
	findings2 []string
}

// declare writes vibe.yaml declaring version, with the case's entrypoints.
func (c byPathCase) declare(version string) contracttest.Edit {
	sections := []contracttest.Section{{Key: "contract", Body: "contract: " + version + "\n"}}
	if c.entrypoints != "" {
		sections = append(sections, contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n" + c.entrypoints})
	}
	return contracttest.WithFile("vibe.yaml", contracttest.ConfigWith(sections...))
}

// runByPath runs each case under 1.2.0 and 2.0.0 and checks the part of
// rule id that kind names.
func runByPath(t *testing.T, id string, kind contract.PartKind, cases []byPathCase) {
	t.Helper()
	for _, tc := range cases {
		for _, v := range []struct {
			version  string
			want     contract.Result
			findings []string
		}{{contract.Contract1, tc.v1, nil}, {contract.Contract2, tc.v2, tc.findings2}} {
			t.Run(tc.name+"/"+v.version, func(t *testing.T) {
				t.Parallel()
				edits := append(append([]contracttest.Edit{}, tc.edits...), tc.declare(v.version))
				rep := contracttest.Audit(t, contracttest.Fixture(t, contracttest.Baseline, edits...), contract.Options{Static: true})
				contracttest.AssertPart(t, rep, id, kind, v.want, v.findings...)
			})
		}
	}
}

// byPathEntrypoints declares the baseline entrypoints and a setup
// entrypoint that runs cmd.
func byPathEntrypoints(cmd string) string {
	return "  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n" +
		"  setup: |-\n    " + cmd + "\n"
}

// byPathStep replaces ci.yml with a verify job and a job x that runs cmd.
func byPathStep(cmd string) contracttest.Edit {
	return ciWith(ciPRAndPush, ciVerifyJob("v", "ubuntu-latest")+ciJob("x", "ubuntu-latest", "|\n          "+cmd))
}

// byPathHook writes a husky pre-commit hook that runs cmd.
func byPathHook(cmd string) contracttest.Edit {
	return contracttest.WithFile(".husky/pre-commit", cmd+"\n")
}

// The calls of vibe by path that 2.0.0 counts. The last path element is
// read from the word as written, so an unquoted backslash separates
// elements too, although POSIX quote removal drops it.
var vibeByPath = []string{`"$dir/vibe" verify`, `./bin/vibe verify`, `'C:\tools\vibe.exe' verify`, `C:\tools\vibe.exe verify`,
	`${RUNNER_TEMP}/bin/vibe verify`}

// Words that are not a call of vibe under either contract.
var notVibe = []string{`"$dir/vibe-guard" hook`, `"$bin" verify`, `./bin/VIBE.EXE verify`, `"$dir/vibe$suffix" verify`, `./vibe/run verify`}

func TestRuleR01VibeByPath(t *testing.T) {
	t.Parallel()
	var cases []byPathCase
	for _, cmd := range vibeByPath {
		cases = append(cases,
			byPathCase{name: "entrypoint " + cmd, entrypoints: byPathEntrypoints(cmd), v1: contract.ResultPass, v2: contract.ResultFail,
				findings2: []string{"vibe.yaml: entrypoints.setup: ", "calls vibe"}},
			byPathCase{name: "hook " + cmd, edits: []contracttest.Edit{byPathHook(cmd)}, v1: contract.ResultPass, v2: contract.ResultFail,
				findings2: []string{".husky/pre-commit: ", "calls vibe"}},
			byPathCase{name: "CI step " + cmd, edits: []contracttest.Edit{byPathStep(cmd)}, v1: contract.ResultPass, v2: contract.ResultFail,
				findings2: []string{ciFile + ": ", "job x", "calls vibe"}})
	}
	for _, cmd := range notVibe {
		cases = append(cases,
			byPathCase{name: "entrypoint " + cmd, entrypoints: byPathEntrypoints(cmd), v1: contract.ResultPass, v2: contract.ResultPass},
			byPathCase{name: "hook " + cmd, edits: []contracttest.Edit{byPathHook(cmd)}, v1: contract.ResultPass, v2: contract.ResultPass},
			byPathCase{name: "CI step " + cmd, edits: []contracttest.Edit{byPathStep(cmd)}, v1: contract.ResultPass, v2: contract.ResultPass})
	}
	cases = append(cases,
		byPathCase{name: "vibe by name", edits: []contracttest.Edit{byPathStep("vibe verify")}, v1: contract.ResultFail, v2: contract.ResultFail},
		// Quote removal makes vi\be the word vibe: a call by name, whose
		// backslash does not separate path elements.
		byPathCase{name: "vibe by name with an escaped letter", edits: []contracttest.Edit{byPathStep(`vi\be verify`)},
			v1: contract.ResultFail, v2: contract.ResultFail},
		// vibe --version by path tests an installed binary too (spec 0055).
		byPathCase{name: "vibe --version by path in a job that calls no entrypoint",
			edits: []contracttest.Edit{byPathStep(`"$dir/vibe" --version`)}, v1: contract.ResultPass, v2: contract.ResultPass},
		byPathCase{name: "vibe --version by path in a job that calls verify",
			edits: []contracttest.Edit{ciWith(ciPRAndPush, ciJob("v", "ubuntu-latest", "./bin/vibe --version", "task verify"))},
			v1:    contract.ResultPass, v2: contract.ResultFail, findings2: []string{"job v", "`./bin/vibe --version`"}},
		byPathCase{name: "the audit job runs vibe audit by path",
			edits: []contracttest.Edit{ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", "|\n          \"$dir/vibe\" doctor && \"$dir/vibe\" audit"))},
			v1:    contract.ResultPass, v2: contract.ResultPass},
	)
	runByPath(t, "R-01", contract.Static, cases)
}

func TestRuleC04VibeByPath(t *testing.T) {
	t.Parallel()
	audit := func(cmds ...string) []contracttest.Edit {
		return []contracttest.Edit{ciAuditWith(ciPRAndPush, ciJob("audit", "ubuntu-latest", cmds...))}
	}
	runByPath(t, "C-04", contract.Static, []byPathCase{
		{name: "vibe audit by path", edits: audit(`|
          "$dir/vibe" audit`), v1: contract.ResultFail, v2: contract.ResultPass},
		{name: "vibe.exe audit by path", edits: audit(`|
          'C:\tools\vibe.exe' audit --json`), v1: contract.ResultFail, v2: contract.ResultPass},
		{name: "vibe.exe audit by an unquoted path", edits: audit(`|
          C:\tools\vibe.exe audit --json`), v1: contract.ResultFail, v2: contract.ResultPass},
		{name: "vibe audit by path beside verify", edits: audit(`./bin/vibe audit`, "task verify"),
			v1: contract.ResultFail, v2: contract.ResultFail, findings2: []string{"job audit", "`task verify`"}},
		{name: "vibe-guard audit by path", edits: audit(`./bin/vibe-guard audit`), v1: contract.ResultFail, v2: contract.ResultFail},
		{name: "the last element holds an expansion", edits: audit(`|
          "$dir/$tool" audit`), v1: contract.ResultFail, v2: contract.ResultFail},
	})
}
