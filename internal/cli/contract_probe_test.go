package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// The probe flags of vibe audit and how it renders what the guard probes
// report (spec 0053). The probe and guard probe parts themselves are
// tested through contract.Audit in internal/contract (probe_test.go,
// probe_light_test.go, guardprobe_reporting_test.go and
// guardprobe_test.go).

func TestContractAuditProbeTimeoutMustBePositive(t *testing.T) {
	t.Parallel()
	r := runVibeAudit(t, "--repo-root", contractFixture(t), "--probe-timeout", "0s")
	assertExit(t, r, 1)
	assertStderr(t, r, "--probe-timeout must be a positive duration")
}

func TestContractAuditProbeFlagsOnV1PointAtTheGuide(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeVibeYAML(t, dir, "standard: prod-go\nversion: v1\n")
	for _, flag := range [][]string{{"--probe-timeout", "1m"}, {"--keep-probe-copy"}} {
		r := runVibeAudit(t, append([]string{"--repo-root", dir}, flag...)...)
		assertExit(t, r, 1)
		assertStderr(t, r, "version 1", migrationGuide)
	}
}

// --keep-probe-copy keeps the copy and prints its path on stderr, and the
// progress lines go to stderr in text mode only. The fixture runs no
// program: the entrypoints' runs announce themselves before they find no
// tool on the empty PATH.
func TestContractAuditProbeFlagsReachTheAudit(t *testing.T) {
	t.Parallel()
	dir := contractFixture(t)
	t.Run("text mode prints progress lines and --keep-probe-copy the kept copy's path", func(t *testing.T) {
		t.Parallel()
		r := runVibeAudit(t, "--repo-root", dir, "--keep-probe-copy")
		assertStderr(t, r, "vibe audit: verify", "vibe audit: keeping the probe copy at ")
		_, kept, _ := strings.Cut(r.stderr, "vibe audit: keeping the probe copy at ")
		kept, _, _ = strings.Cut(kept, "\n")
		// The copy is kept in the audit's temporary directory, which the
		// test removes.
		tmp := filepath.Dir(kept)
		if !strings.HasPrefix(filepath.Base(tmp), "vibe-audit-") {
			t.Fatalf("the kept copy %q is not in a vibe-audit- directory", kept)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmp) })
		if _, err := os.Stat(filepath.Join(kept, "go.mod")); err != nil {
			t.Errorf("the kept copy lacks a tracked file: %v", err)
		}
	})
	t.Run("--json prints no progress lines", func(t *testing.T) {
		t.Parallel()
		if r := runVibeAudit(t, "--repo-root", dir, "--json"); strings.Contains(r.stderr, "vibe audit: ") {
			t.Errorf("--json printed progress lines:\n%s", r.stderr)
		}
	})
}

// rmBuildUses is a report whose one guard exception let five corpus
// cases through.
func rmBuildUses(t *testing.T) (*contract.Report, []string) {
	t.Helper()
	cases := []string{
		"rm -r -f (spec 0047)",
		"rm with extra blanks",
		"variant (#105): quoted flags",
		"variant (#105): sudo with options, by full paths",
		"variant (#105): sh -c with separate flags",
	}
	rep := reportOf(t, false, nil)
	rep.GuardExceptions = []contract.GuardException{{Rule: "rm-rf", Match: "rm -r -f build", Reason: "the build directory is disposable"}}
	for _, c := range cases {
		rep.GuardExceptionUses = append(rep.GuardExceptionUses, contract.GuardExceptionUse{Rule: "rm-rf", Match: "rm -r -f build", Case: c})
	}
	return rep, cases
}

func renderJSON(t *testing.T, rep *contract.Report) string {
	t.Helper()
	var b strings.Builder
	if err := writeContractJSON(&b, rep, "test"); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// How the report renders the corpus cases each guard exception let
// through (A-02). TestRuleA02Reporting in internal/contract checks that
// the audit reports them.
func TestContractReportGuardExceptionUses(t *testing.T) {
	t.Parallel()
	t.Run("an exception's cases are listed in JSON and text", func(t *testing.T) {
		t.Parallel()
		rep, rmBuildCases := rmBuildUses(t)
		var out struct {
			Uses []struct {
				Rule  string `json:"rule"`
				Match string `json:"match"`
				Case  string `json:"case"`
			} `json:"guard_exception_uses"`
		}
		if err := json.Unmarshal([]byte(renderJSON(t, rep)), &out); err != nil {
			t.Fatalf("no JSON: %v", err)
		}
		var cases []string
		for _, u := range out.Uses {
			if u.Rule != "rm-rf" || u.Match != "rm -r -f build" {
				t.Errorf("a use names exception %s %q", u.Rule, u.Match)
			}
			cases = append(cases, u.Case)
		}
		if !slices.Equal(cases, rmBuildCases) {
			t.Errorf("guard_exception_uses cases = %q, want %q", cases, rmBuildCases)
		}

		text := renderText(t, rep, "test")
		want := "Guard exceptions\n" +
			"  rm-rf \"rm -r -f build\" (the build directory is disposable)\n"
		for _, c := range rmBuildCases {
			want += "    let through: " + c + "\n"
		}
		if !strings.Contains(text, want) {
			t.Errorf("the report lacks\n%s\nstdout:\n%s", want, text)
		}
	})
	t.Run("without exceptions guard_exception_uses is an empty list", func(t *testing.T) {
		t.Parallel()
		if out := renderJSON(t, reportOf(t, false, nil)); !strings.Contains(out, `"guard_exception_uses": []`) {
			t.Errorf("--json lacks an empty guard_exception_uses:\n%s", out)
		}
	})
}
