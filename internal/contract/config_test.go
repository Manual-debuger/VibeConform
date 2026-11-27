package contract_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
	"github.com/Manual-debuger/VibeConform/internal/contract/contracttest"
)

// configLoadCase is one invalid (or valid) configuration and what
// contract.Load's error must name. A valid case loads and reaches the
// rule engine. The CLI prints the error as it is, after "Error: ", and
// exits 1 (TestAuditConfigErrorsNameTheConfigFile in internal/cli).
type configLoadCase struct {
	name   string
	config string
	edits  []contracttest.Edit
	want   []string // substrings of the load error; nil means valid
}

func runConfigLoadCases(t *testing.T, cases []configLoadCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			edits := append([]contracttest.Edit{contracttest.WithFile("vibe.yaml", tc.config)}, tc.edits...)
			dir := contracttest.Fixture(t, contracttest.Baseline, edits...)
			if tc.want == nil {
				// Static: these cases test the configuration, which the
				// probe copy adds nothing to. Audit fails the test if
				// the configuration does not load.
				contracttest.Audit(t, dir, contract.Options{Static: true})
				return
			}
			assertConfigLoadError(t, dir, tc.want...)
		})
	}
}

// assertConfigLoadError loads the configuration at dir and checks that
// it is refused with an error that names the configuration, before the
// rule engine is reached, and holds each of want.
func assertConfigLoadError(t *testing.T, dir string, want ...string) {
	t.Helper()
	cfg, _, err := contract.Load(context.Background(), dir, "", contracttest.OpenRepo)
	if err == nil {
		t.Fatalf("the configuration loaded, want an error naming %q", want)
	}
	if cfg != nil {
		t.Errorf("an invalid configuration reached the rule engine: %v", err)
	}
	var ce *contract.ConfigError
	var ve *contract.UnsupportedVersionError
	if !errors.As(err, &ce) && !errors.As(err, &ve) {
		t.Errorf("err = %T %v, want a *ConfigError or an *UnsupportedVersionError", err, err)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("load error missing %q\nerror: %v", w, err)
		}
	}
}

func TestContractConfigTopLevel(t *testing.T) {
	t.Parallel()
	runConfigLoadCases(t, []configLoadCase{
		{name: "unknown field", config: contracttest.ConfigWith(contracttest.Section{Key: "lint", Body: "lint: true\n"}),
			want: []string{"vibe.yaml: lint: unknown field"}},
		{name: "misspelled field", config: contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platform: [linux]\n"}),
			want: []string{"vibe.yaml: platform: unknown field"}},
		{name: "duplicate field", config: contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: "agents: [claude]\nagents: []\n"}),
			want: []string{"vibe.yaml: agents: ", "more than once"}},
		{name: "two documents", config: contracttest.ConfigWith() + "---\ncontract: 1.0.0\n",
			want: []string{"vibe.yaml: ", "more than one YAML document"}},
		{name: "invalid YAML", config: contracttest.ConfigWith() + "platforms: [linux\n",
			want: []string{"vibe.yaml: ", "not valid YAML"}},
	})
}

// The v1 fields say where their content went (spec 0052, Q23).
func TestContractConfigV1Fields(t *testing.T) {
	t.Parallel()
	runConfigLoadCases(t, []configLoadCase{
		{name: "version", config: contracttest.ConfigWith(contracttest.Section{Key: "version", Body: "version: v1\n"}),
			want: []string{"vibe.yaml: version: ", "no version field", "contract"}},
		{name: "integrations", config: contracttest.ConfigWith(contracttest.Section{Key: "integrations", Body: "integrations:\n  agents: [claude]\n"}),
			want: []string{"vibe.yaml: integrations: ", "no integrations field", "agents", "setup skill", "repository intelligence has no counterpart"}},
		{name: "development", config: contracttest.ConfigWith(contracttest.Section{Key: "development", Body: "development:\n  workflow: plan-triggered-sdd\n"}),
			want: []string{"vibe.yaml: development: ", "no development field", "setup skill"}},
		{name: "policy", config: contracttest.ConfigWith(contracttest.Section{Key: "policy", Body: "policy:\n  line_endings: lf\n"}),
			want: []string{"vibe.yaml: policy: ", "no policy field", "setup skill"}},
		{name: "ci", config: contracttest.ConfigWith(contracttest.Section{Key: "ci", Body: "ci:\n  provider: github\n"}),
			want: []string{"vibe.yaml: ci: ", "no ci field", "GitHub Actions"}},
	})
}

func TestContractConfigVersion(t *testing.T) {
	t.Parallel()
	runConfigLoadCases(t, []configLoadCase{
		{name: "not X.Y.Z", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: \"1.0\"\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "leading v", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: v1.0.0\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "leading zero", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.00.0\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "not a string", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: [1, 0, 0]\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "newer MAJOR", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 3.0.0\n"}),
			want: []string{"vibe.yaml: contract: ", "cannot decide; upgrade vibe", "3.0.0", "1.2.0", "2.1.0"}},
		{name: "newer MINOR of 2", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 2.2.0\n"}),
			want: []string{"vibe.yaml: contract: ", "cannot decide; upgrade vibe", "2.2.0", "2.1.0"}},
		{name: "contract 2.1.0 is audited", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 2.1.0\n"})},
		{name: "older MINOR of 2 is audited", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 2.0.0\n"})},
		{name: "newer PATCH of 2 is audited", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 2.0.3\n"})},
		{name: "older MAJOR", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 0.9.0\n"}),
			want: []string{"vibe.yaml: contract: ", "cannot decide; upgrade vibe"}},
		{name: "newer MINOR", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.3.0\n"}),
			want: []string{"vibe.yaml: contract: ", "cannot decide; upgrade vibe", "1.3.0", "1.2.0"}},
		// A newer MINOR may add fields this vibe does not know: the version
		// decides before the unknown field does.
		{name: "newer MINOR with a new field", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.3.0\n"}, contracttest.Section{Key: "future", Body: "future: true\n"}),
			want: []string{"cannot decide; upgrade vibe"}},
		{name: "newer PATCH is audited", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.2.7\n"})},
		// A repository set up under contract 1.0.x or 1.1.x keeps its
		// declaration and is audited with 1.2.0 (spec 0051, "Versioning";
		// spec 0061, "Contract version").
		{name: "older MINOR is audited", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.0.0\n"})},
		{name: "older MINOR's PATCH is audited", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.0.1\n"})},
		{name: "contract 1.1.0 is audited", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 1.1.0\n"})},
	})
}

func TestContractConfigPlatformsAndAgents(t *testing.T) {
	t.Parallel()
	runConfigLoadCases(t, []configLoadCase{
		{name: "platforms missing", config: contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: ""}),
			want: []string{"vibe.yaml: platforms: is required"}},
		{name: "platforms empty", config: contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: []\n"}),
			want: []string{"vibe.yaml: platforms: ", "at least one"}},
		{name: "platforms not a list", config: contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: linux\n"}),
			want: []string{"vibe.yaml: platforms: ", "list"}},
		{name: "platform unknown", config: contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: [linux, bsd]\n"}),
			want: []string{"vibe.yaml: platforms[1]: ", "bsd", "linux, windows, macos"}},
		{name: "platform twice", config: contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: [linux, windows, linux]\n"}),
			want: []string{"vibe.yaml: platforms[2]: ", "twice"}},
		{name: "platform not a string", config: contracttest.ConfigWith(contracttest.Section{Key: "platforms", Body: "platforms: [linux, {os: windows}]\n"}),
			want: []string{"vibe.yaml: platforms[1]: "}},
		{name: "agents missing", config: contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: ""}),
			want: []string{"vibe.yaml: agents: is required"}},
		{name: "agents empty is valid", config: contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: "agents: []\n"})},
		{name: "agents null", config: contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: "agents:\n"}),
			want: []string{"vibe.yaml: agents: ", "list"}},
		{name: "agent unknown", config: contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: "agents: [claude, cursor]\n"}),
			want: []string{"vibe.yaml: agents[1]: ", "cursor", "claude, codex"}},
		{name: "agent twice", config: contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: "agents: [codex, codex]\n"}),
			want: []string{"vibe.yaml: agents[1]: ", "twice"}},
		{name: "codex is valid", config: contracttest.ConfigWith(contracttest.Section{Key: "agents", Body: "agents: [claude, codex]\n"})},
	})
}

func TestContractConfigEntrypoints(t *testing.T) {
	t.Parallel()
	runConfigLoadCases(t, []configLoadCase{
		{name: "not a map", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints: task verify\n"}),
			want: []string{"vibe.yaml: entrypoints: ", "map"}},
		{name: "unknown entrypoint", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  verify: task verify\n  test: task test\n"}),
			want: []string{"vibe.yaml: entrypoints.test: ", "verify, verify-fast, fmt, fmt-check, setup"}},
		{name: "value is a map", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  verify:\n    run: task verify\n"}),
			want: []string{"vibe.yaml: entrypoints.verify: ", "a command or a list of commands"}},
		{name: "value is a number", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  fmt: 3\n"}),
			want: []string{"vibe.yaml: entrypoints.fmt: ", "a command or a list of commands"}},
		{name: "list item not a string", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  verify: [task lint, task test, [task x]]\n"}),
			want: []string{"vibe.yaml: entrypoints.verify[2]: ", "string"}},
		{name: "duplicate entrypoint", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  fmt: task fmt\n  fmt: task fmt2\n"}),
			want: []string{"vibe.yaml: entrypoints.fmt: ", "more than once"}},
		// A missing or empty required entrypoint fails V-04, not the
		// configuration (spec 0052): the rest of the report still runs.
		{name: "missing entrypoint is valid", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  verify: task verify\n"})},
		{name: "empty entrypoint is valid", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  verify: []\n  fmt: \"\"\n  fmt-check:\n"})},
		{name: "no entrypoints is valid", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: ""})},
		{name: "list and setup are valid", config: contracttest.ConfigWith(contracttest.Section{Key: "entrypoints", Body: "entrypoints:\n  setup: task setup\n  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"})},
	})
}

func TestContractConfigWaivers(t *testing.T) {
	t.Parallel()
	waiver := func(body string) contracttest.Section {
		return contracttest.Section{Key: "waivers", Body: "waivers:\n" + body}
	}
	runConfigLoadCases(t, []configLoadCase{
		{name: "valid", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: lint is platform-independent\n    until: 2026-12-31\n"))},
		{name: "not a list", config: contracttest.ConfigWith(contracttest.Section{Key: "waivers", Body: "waivers: C-05\n"}),
			want: []string{"vibe.yaml: waivers: ", "list"}},
		{name: "entry not a map", config: contracttest.ConfigWith(waiver("  - C-05\n")),
			want: []string{"vibe.yaml: waivers[0]: ", "rule, reason and until"}},
		{name: "unknown field", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: r\n    until: 2026-12-31\n    by: eric\n")),
			want: []string{"vibe.yaml: waivers[0].by: unknown field"}},
		{name: "rule missing", config: contracttest.ConfigWith(waiver("  - reason: r\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].rule: is required"}},
		{name: "reason missing", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].reason: is required"}},
		{name: "until missing", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: r\n")),
			want: []string{"vibe.yaml: waivers[0].until: is required"}},
		{name: "unknown rule", config: contracttest.ConfigWith(waiver("  - rule: X-99\n    reason: r\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].rule: ", "X-99", "not a rule of contract 1.2.0"}},
		{name: "MUST rule", config: contracttest.ConfigWith(waiver("  - rule: C-02\n    reason: r\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].rule: ", "C-02 is a MUST rule", "SHOULD"}},
		{name: "blank reason", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: \"   \"\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].reason: ", "empty"}},
		{name: "reason not a string", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: [a]\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].reason: "}},
		{name: "until not a date", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: r\n    until: next year\n")),
			want: []string{"vibe.yaml: waivers[0].until: ", "YYYY-MM-DD"}},
		{name: "until not a calendar day", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: r\n    until: 2026-02-30\n")),
			want: []string{"vibe.yaml: waivers[0].until: ", "YYYY-MM-DD"}},
		{name: "until with a time", config: contracttest.ConfigWith(waiver("  - rule: C-05\n    reason: r\n    until: 2026-12-31T10:00:00Z\n")),
			want: []string{"vibe.yaml: waivers[0].until: ", "YYYY-MM-DD"}},
		{name: "second waiver for a rule", config: contracttest.ConfigWith(waiver(
			"  - rule: C-05\n    reason: r\n    until: 2026-12-31\n  - rule: C-05\n    reason: s\n    until: 2027-01-31\n")),
			want: []string{"vibe.yaml: waivers[1].rule: ", "waivers[0]"}},
	})
}

// require lists the SHOULD rules a repository holds itself to (spec 0063).
func TestContractConfigRequire(t *testing.T) {
	t.Parallel()
	require := func(body string) contracttest.Section {
		return contracttest.Section{Key: "require", Body: "require: " + body + "\n"}
	}
	waiver := func(rule, until string) contracttest.Section {
		return contracttest.Section{Key: "waivers", Body: "waivers:\n  - rule: " + rule + "\n    reason: r\n    until: " + until + "\n"}
	}
	runConfigLoadCases(t, []configLoadCase{
		{name: "valid", config: contracttest.ConfigWith(require("[V-05, V-06, A-09]"))},
		{name: "empty is valid", config: contracttest.ConfigWith(require("[]"))},
		{name: "beside a waiver for another rule", config: contracttest.ConfigWith(waiver("R-03", "2999-12-31"), require("[V-06]"))},
		{name: "not a list", config: contracttest.ConfigWith(require("V-06")),
			want: []string{"vibe.yaml: require: ", "list of SHOULD rule IDs"}},
		{name: "unknown rule", config: contracttest.ConfigWith(require("[V-06, X-99]")),
			want: []string{"vibe.yaml: require[1]: ", "X-99", "not a rule of contract 1.2.0"}},
		{name: "item not a string", config: contracttest.ConfigWith(require("[V-06, {rule: V-07}]")),
			want: []string{"vibe.yaml: require[1]: ", "not a rule of contract 1.2.0"}},
		{name: "MUST rule", config: contracttest.ConfigWith(require("[C-02]")),
			want: []string{"vibe.yaml: require[0]: C-02 is a MUST rule; it is always required"}},
		{name: "listed twice", config: contracttest.ConfigWith(require("[V-06, V-07, V-06]")),
			want: []string{"vibe.yaml: require[2]: ", "V-06 is listed twice (also require[0])"}},
		{name: "waived and required", config: contracttest.ConfigWith(waiver("V-06", "2999-12-31"), require("[V-07, V-06]")),
			want: []string{"vibe.yaml: require[1]: V-06 is both required and waived (waivers[0])"}},
		{name: "required before its waiver", config: contracttest.ConfigWith(require("[V-06]"), waiver("V-06", "2999-12-31")),
			want: []string{"vibe.yaml: require[0]: V-06 is both required and waived (waivers[0])"}},
		{name: "required and waived by an expired waiver", config: contracttest.ConfigWith(waiver("V-06", "2000-01-01"), require("[V-06]")),
			want: []string{"vibe.yaml: require[0]: V-06 is both required and waived (waivers[0])"}},
	})
}

// Under contract 2.1.0 the guard set is SHOULD, and require lists all of
// it or none of it; require with A-09 needs the whole set. Under 1.x the
// guard set is MUST, so require and waivers reject it (spec 0074).
func TestContractConfigGuardSet(t *testing.T) {
	t.Parallel()
	v2 := contracttest.Section{Key: "contract", Body: "contract: 2.1.0\n"}
	require := func(body string) contracttest.Section {
		return contracttest.Section{Key: "require", Body: "require: " + body + "\n"}
	}
	waiver := func(rule string) contracttest.Section {
		return contracttest.Section{Key: "waivers", Body: "waivers:\n  - rule: " + rule + "\n    reason: r\n    until: 2999-12-31\n"}
	}
	runConfigLoadCases(t, []configLoadCase{
		{name: "2.1.0 whole set", config: contracttest.ConfigWith(v2, require("[A-01, A-02, A-03, A-04, A-08]"))},
		{name: "2.1.0 whole set in another order, with A-09", config: contracttest.ConfigWith(v2, require("[V-06, A-09, A-08, A-04, A-03, A-02, A-01]"))},
		{name: "2.0.0 whole set", config: contracttest.ConfigWith(contracttest.Section{Key: "contract", Body: "contract: 2.0.0\n"}, require("[A-01, A-02, A-03, A-04, A-08]"))},
		{name: "2.1.0 no guard set", config: contracttest.ConfigWith(v2, require("[V-06]"))},
		{name: "2.1.0 part of the set", config: contracttest.ConfigWith(v2, require("[V-06, A-01, A-02]")),
			want: []string{"vibe.yaml: require: ", "part of the guard set", "add A-03, A-04 and A-08"}},
		{name: "2.1.0 one rule of the set", config: contracttest.ConfigWith(v2, require("[A-08]")),
			want: []string{"vibe.yaml: require: ", "part of the guard set", "add A-01, A-02, A-03 and A-04"}},
		{name: "2.1.0 A-09 without the set", config: contracttest.ConfigWith(v2, require("[V-06, A-09]")),
			want: []string{"vibe.yaml: require[1]: ", "A-09 needs the guard set", "add A-01, A-02, A-03, A-04 and A-08"}},
		{name: "2.1.0 A-09 with part of the set", config: contracttest.ConfigWith(v2, require("[A-09, A-01]")),
			want: []string{"vibe.yaml: require: ", "part of the guard set", "add A-02, A-03, A-04 and A-08"}},
		{name: "1.2.0 the set", config: contracttest.ConfigWith(require("[A-01, A-02, A-03, A-04, A-08]")),
			want: []string{"vibe.yaml: require[0]: A-01 is a MUST rule; it is always required"}},
		{name: "1.2.0 A-09 alone", config: contracttest.ConfigWith(require("[A-09]"))},
		{name: "2.1.0 waiver of a guard rule", config: contracttest.ConfigWith(v2, waiver("A-02"))},
		{name: "2.1.0 waiver of a required guard rule", config: contracttest.ConfigWith(v2, waiver("A-02"), require("[A-01, A-02, A-03, A-04, A-08]")),
			want: []string{"vibe.yaml: require[1]: A-02 is both required and waived (waivers[0])"}},
		{name: "1.2.0 waiver of a guard rule", config: contracttest.ConfigWith(waiver("A-02")),
			want: []string{"vibe.yaml: waivers[0].rule: ", "A-02 is a MUST rule", "SHOULD"}},
	})
}

// GuardSet is the guard set in the order spec 0074 names it.
func TestGuardSet(t *testing.T) {
	t.Parallel()
	want := []string{"A-01", "A-02", "A-03", "A-04", "A-08"}
	got := contract.GuardSet()
	if !slices.Equal(got, want) {
		t.Fatalf("GuardSet() = %v, want %v", got, want)
	}
	got[0] = "X-99"
	if contract.GuardSet()[0] != "A-01" {
		t.Error("GuardSet() returned the package's own slice")
	}
	for _, id := range want {
		r, ok := contract.LookupRule(id)
		if !ok {
			t.Fatalf("%s is not a rule", id)
		}
		if r.LevelAt(contract.Contract1) != contract.Must || r.LevelAt(contract.Contract2) != contract.Should {
			t.Errorf("%s level = %s under %s and %s under %s, want MUST and SHOULD", id,
				r.LevelAt(contract.Contract1), contract.Contract1, r.LevelAt(contract.Contract2), contract.Contract2)
		}
	}
}

// The parser reports the first error in document order.
func TestContractConfigFirstErrorInDocumentOrder(t *testing.T) {
	t.Parallel()
	badPlatform := contracttest.Section{Key: "platforms", Body: "platforms: [bsd]\n"}
	badWaiver := contracttest.Section{Key: "waivers", Body: "waivers:\n  - rule: C-02\n    reason: r\n    until: 2026-12-31\n"}
	firstError := func(t *testing.T, config string) *contract.ConfigError {
		t.Helper()
		dir := contracttest.Fixture(t, contracttest.Baseline, contracttest.WithFile("vibe.yaml", config))
		_, _, err := contract.Load(context.Background(), dir, "", contracttest.OpenRepo)
		var ce *contract.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %T %v, want a *ConfigError", err, err)
		}
		return ce
	}
	t.Run("platforms first", func(t *testing.T) {
		t.Parallel()
		if ce := firstError(t, contracttest.ConfigWith(badPlatform, badWaiver)); ce.File != "vibe.yaml" || ce.Path != "platforms[0]" {
			t.Errorf("first error = %v, want vibe.yaml at platforms[0], not the later waiver", ce)
		}
	})
	t.Run("waivers first", func(t *testing.T) {
		t.Parallel()
		if ce := firstError(t, badWaiver.Body+contracttest.ConfigWith(badPlatform)); ce.File != "vibe.yaml" || ce.Path != "waivers[0].rule" {
			t.Errorf("first error = %v, want vibe.yaml at waivers[0].rule, not the later platform", ce)
		}
	})
}
