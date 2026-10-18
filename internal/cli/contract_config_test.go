package cli

import (
	"strings"
	"testing"
)

// section is one top-level field of a fixture vibe.yaml, with its text.
type section struct {
	key  string
	body string
}

// baseConfig is the baseline fixture's vibe.yaml, field by field.
var baseConfig = []section{
	{"contract", "contract: 1.0.0\n"},
	{"platforms", "platforms: [linux, windows]\n"},
	{"agents", "agents: [claude]\n"},
	{"entrypoints", "entrypoints:\n  verify: task verify\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"},
}

// configWith is the baseline vibe.yaml with fields replaced in place (an
// empty body removes the field) and new fields appended in order.
func configWith(changes ...section) string {
	var b strings.Builder
	used := map[string]bool{}
	for _, s := range baseConfig {
		body := s.body
		for _, c := range changes {
			if c.key == s.key {
				body = c.body
				used[c.key] = true
			}
		}
		b.WriteString(body)
	}
	for _, c := range changes {
		if !used[c.key] {
			b.WriteString(c.body)
		}
	}
	return b.String()
}

// configCase is one invalid (or valid) configuration and what stderr must
// name. A valid case reaches the rule engine and prints a report; no
// fixture reaches a verdict in Phase 1, so it still exits 1.
type configCase struct {
	name   string
	config string
	edits  []fixtureEdit
	want   []string // substrings of stderr; nil means valid
}

func runConfigCases(t *testing.T, cases []configCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			edits := append([]fixtureEdit{withFile("vibe.yaml", tc.config)}, tc.edits...)
			dir := contractFixture(t, edits...)
			// --static: these cases test the configuration, which the
			// probe copy adds nothing to; it exits 1 either way.
			r := runVibeAudit(t, "--repo-root", dir, "--static")
			assertExit(t, r, 1)
			if tc.want == nil {
				assertReport(t, r)
				if strings.Contains(r.stderr, "vibe.yaml:") {
					t.Errorf("valid configuration reported an error:\n%s", r.stderr)
				}
				return
			}
			assertStderr(t, r, tc.want...)
			if r.stdout != "" {
				t.Errorf("invalid configuration reached the rule engine:\n%s", r.stderr)
			}
		})
	}
}

func TestContractConfigTopLevel(t *testing.T) {
	t.Parallel()
	runConfigCases(t, []configCase{
		{name: "unknown field", config: configWith(section{"lint", "lint: true\n"}),
			want: []string{"vibe.yaml: lint: unknown field"}},
		{name: "misspelled field", config: configWith(section{"platforms", "platform: [linux]\n"}),
			want: []string{"vibe.yaml: platform: unknown field"}},
		{name: "duplicate field", config: configWith(section{"agents", "agents: [claude]\nagents: []\n"}),
			want: []string{"vibe.yaml: agents: ", "more than once"}},
		{name: "two documents", config: configWith() + "---\ncontract: 1.0.0\n",
			want: []string{"vibe.yaml: ", "more than one YAML document"}},
		{name: "invalid YAML", config: configWith() + "platforms: [linux\n",
			want: []string{"vibe.yaml: ", "not valid YAML"}},
	})
}

// The v1 fields say where their content went (spec 0052, Q23).
func TestContractConfigV1Fields(t *testing.T) {
	t.Parallel()
	runConfigCases(t, []configCase{
		{name: "version", config: configWith(section{"version", "version: v1\n"}),
			want: []string{"vibe.yaml: version: ", "no version field", "contract"}},
		{name: "integrations", config: configWith(section{"integrations", "integrations:\n  agents: [claude]\n"}),
			want: []string{"vibe.yaml: integrations: ", "no integrations field", "agents", "setup skill"}},
		{name: "development", config: configWith(section{"development", "development:\n  workflow: plan-triggered-sdd\n"}),
			want: []string{"vibe.yaml: development: ", "no development field", "setup skill"}},
		{name: "policy", config: configWith(section{"policy", "policy:\n  line_endings: lf\n"}),
			want: []string{"vibe.yaml: policy: ", "no policy field", "setup skill"}},
		{name: "ci", config: configWith(section{"ci", "ci:\n  provider: github\n"}),
			want: []string{"vibe.yaml: ci: ", "no ci field", "GitHub Actions"}},
	})
}

func TestContractConfigVersion(t *testing.T) {
	t.Parallel()
	runConfigCases(t, []configCase{
		{name: "not X.Y.Z", config: configWith(section{"contract", "contract: \"1.0\"\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "leading v", config: configWith(section{"contract", "contract: v1.0.0\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "leading zero", config: configWith(section{"contract", "contract: 1.00.0\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "not a string", config: configWith(section{"contract", "contract: [1, 0, 0]\n"}),
			want: []string{"vibe.yaml: contract: ", "X.Y.Z"}},
		{name: "newer MAJOR", config: configWith(section{"contract", "contract: 2.0.0\n"}),
			want: []string{"vibe.yaml: contract: ", "cannot decide; upgrade vibe", "2.0.0", "1.0.0"}},
		{name: "older MAJOR", config: configWith(section{"contract", "contract: 0.9.0\n"}),
			want: []string{"vibe.yaml: contract: ", "cannot decide; upgrade vibe"}},
		{name: "newer MINOR", config: configWith(section{"contract", "contract: 1.1.0\n"}),
			want: []string{"vibe.yaml: contract: ", "cannot decide; upgrade vibe", "1.1.0"}},
		// A newer MINOR may add fields this vibe does not know: the version
		// decides before the unknown field does.
		{name: "newer MINOR with a new field", config: configWith(section{"contract", "contract: 1.1.0\n"}, section{"future", "future: true\n"}),
			want: []string{"cannot decide; upgrade vibe"}},
		{name: "newer PATCH is audited", config: configWith(section{"contract", "contract: 1.0.7\n"})},
	})
}

func TestContractConfigPlatformsAndAgents(t *testing.T) {
	t.Parallel()
	runConfigCases(t, []configCase{
		{name: "platforms missing", config: configWith(section{"platforms", ""}),
			want: []string{"vibe.yaml: platforms: is required"}},
		{name: "platforms empty", config: configWith(section{"platforms", "platforms: []\n"}),
			want: []string{"vibe.yaml: platforms: ", "at least one"}},
		{name: "platforms not a list", config: configWith(section{"platforms", "platforms: linux\n"}),
			want: []string{"vibe.yaml: platforms: ", "list"}},
		{name: "platform unknown", config: configWith(section{"platforms", "platforms: [linux, bsd]\n"}),
			want: []string{"vibe.yaml: platforms[1]: ", "bsd", "linux, windows, macos"}},
		{name: "platform twice", config: configWith(section{"platforms", "platforms: [linux, windows, linux]\n"}),
			want: []string{"vibe.yaml: platforms[2]: ", "twice"}},
		{name: "platform not a string", config: configWith(section{"platforms", "platforms: [linux, {os: windows}]\n"}),
			want: []string{"vibe.yaml: platforms[1]: "}},
		{name: "agents missing", config: configWith(section{"agents", ""}),
			want: []string{"vibe.yaml: agents: is required"}},
		{name: "agents empty is valid", config: configWith(section{"agents", "agents: []\n"})},
		{name: "agents null", config: configWith(section{"agents", "agents:\n"}),
			want: []string{"vibe.yaml: agents: ", "list"}},
		{name: "agent unknown", config: configWith(section{"agents", "agents: [claude, cursor]\n"}),
			want: []string{"vibe.yaml: agents[1]: ", "cursor", "claude, codex"}},
		{name: "agent twice", config: configWith(section{"agents", "agents: [codex, codex]\n"}),
			want: []string{"vibe.yaml: agents[1]: ", "twice"}},
		{name: "codex is valid", config: configWith(section{"agents", "agents: [claude, codex]\n"})},
	})
}

func TestContractConfigEntrypoints(t *testing.T) {
	t.Parallel()
	runConfigCases(t, []configCase{
		{name: "not a map", config: configWith(section{"entrypoints", "entrypoints: task verify\n"}),
			want: []string{"vibe.yaml: entrypoints: ", "map"}},
		{name: "unknown entrypoint", config: configWith(section{"entrypoints", "entrypoints:\n  verify: task verify\n  test: task test\n"}),
			want: []string{"vibe.yaml: entrypoints.test: ", "verify, verify-fast, fmt, fmt-check, setup"}},
		{name: "value is a map", config: configWith(section{"entrypoints", "entrypoints:\n  verify:\n    run: task verify\n"}),
			want: []string{"vibe.yaml: entrypoints.verify: ", "a command or a list of commands"}},
		{name: "value is a number", config: configWith(section{"entrypoints", "entrypoints:\n  fmt: 3\n"}),
			want: []string{"vibe.yaml: entrypoints.fmt: ", "a command or a list of commands"}},
		{name: "list item not a string", config: configWith(section{"entrypoints", "entrypoints:\n  verify: [task lint, task test, [task x]]\n"}),
			want: []string{"vibe.yaml: entrypoints.verify[2]: ", "string"}},
		{name: "duplicate entrypoint", config: configWith(section{"entrypoints", "entrypoints:\n  fmt: task fmt\n  fmt: task fmt2\n"}),
			want: []string{"vibe.yaml: entrypoints.fmt: ", "more than once"}},
		// A missing or empty required entrypoint fails V-04, not the
		// configuration (spec 0052): the rest of the report still runs.
		{name: "missing entrypoint is valid", config: configWith(section{"entrypoints", "entrypoints:\n  verify: task verify\n"})},
		{name: "empty entrypoint is valid", config: configWith(section{"entrypoints", "entrypoints:\n  verify: []\n  fmt: \"\"\n  fmt-check:\n"})},
		{name: "no entrypoints is valid", config: configWith(section{"entrypoints", ""})},
		{name: "list and setup are valid", config: configWith(section{"entrypoints",
			"entrypoints:\n  setup: task setup\n  verify: [task lint, task test]\n  verify-fast: task verify:fast\n  fmt: task fmt\n  fmt-check: task fmt:check\n"})},
	})
}

func TestContractConfigWaivers(t *testing.T) {
	t.Parallel()
	waiver := func(body string) section { return section{"waivers", "waivers:\n" + body} }
	runConfigCases(t, []configCase{
		{name: "valid", config: configWith(waiver("  - rule: C-05\n    reason: lint is platform-independent\n    until: 2026-12-31\n"))},
		{name: "not a list", config: configWith(section{"waivers", "waivers: C-05\n"}),
			want: []string{"vibe.yaml: waivers: ", "list"}},
		{name: "entry not a map", config: configWith(waiver("  - C-05\n")),
			want: []string{"vibe.yaml: waivers[0]: ", "rule, reason and until"}},
		{name: "unknown field", config: configWith(waiver("  - rule: C-05\n    reason: r\n    until: 2026-12-31\n    by: eric\n")),
			want: []string{"vibe.yaml: waivers[0].by: unknown field"}},
		{name: "rule missing", config: configWith(waiver("  - reason: r\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].rule: is required"}},
		{name: "reason missing", config: configWith(waiver("  - rule: C-05\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].reason: is required"}},
		{name: "until missing", config: configWith(waiver("  - rule: C-05\n    reason: r\n")),
			want: []string{"vibe.yaml: waivers[0].until: is required"}},
		{name: "unknown rule", config: configWith(waiver("  - rule: X-99\n    reason: r\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].rule: ", "X-99", "not a rule of contract 1.0.0"}},
		{name: "MUST rule", config: configWith(waiver("  - rule: C-02\n    reason: r\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].rule: ", "C-02 is a MUST rule", "SHOULD"}},
		{name: "blank reason", config: configWith(waiver("  - rule: C-05\n    reason: \"   \"\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].reason: ", "empty"}},
		{name: "reason not a string", config: configWith(waiver("  - rule: C-05\n    reason: [a]\n    until: 2026-12-31\n")),
			want: []string{"vibe.yaml: waivers[0].reason: "}},
		{name: "until not a date", config: configWith(waiver("  - rule: C-05\n    reason: r\n    until: next year\n")),
			want: []string{"vibe.yaml: waivers[0].until: ", "YYYY-MM-DD"}},
		{name: "until not a calendar day", config: configWith(waiver("  - rule: C-05\n    reason: r\n    until: 2026-02-30\n")),
			want: []string{"vibe.yaml: waivers[0].until: ", "YYYY-MM-DD"}},
		{name: "until with a time", config: configWith(waiver("  - rule: C-05\n    reason: r\n    until: 2026-12-31T10:00:00Z\n")),
			want: []string{"vibe.yaml: waivers[0].until: ", "YYYY-MM-DD"}},
		{name: "second waiver for a rule", config: configWith(waiver(
			"  - rule: C-05\n    reason: r\n    until: 2026-12-31\n  - rule: C-05\n    reason: s\n    until: 2027-01-31\n")),
			want: []string{"vibe.yaml: waivers[1].rule: ", "waivers[0]"}},
	})
}

// The parser reports the first error in document order.
func TestContractConfigFirstErrorInDocumentOrder(t *testing.T) {
	t.Parallel()
	badPlatform := section{"platforms", "platforms: [bsd]\n"}
	badWaiver := section{"waivers", "waivers:\n  - rule: C-02\n    reason: r\n    until: 2026-12-31\n"}
	t.Run("platforms first", func(t *testing.T) {
		dir := contractFixture(t, withFile("vibe.yaml", configWith(badPlatform, badWaiver)))
		r := runVibeAudit(t, "--repo-root", dir)
		assertExit(t, r, 1)
		assertStderr(t, r, "vibe.yaml: platforms[0]:")
		if strings.Contains(r.stderr, "waivers") {
			t.Errorf("reported a later error:\n%s", r.stderr)
		}
	})
	t.Run("waivers first", func(t *testing.T) {
		config := badWaiver.body + configWith(badPlatform)
		dir := contractFixture(t, withFile("vibe.yaml", config))
		r := runVibeAudit(t, "--repo-root", dir)
		assertExit(t, r, 1)
		assertStderr(t, r, "vibe.yaml: waivers[0].rule:")
		if strings.Contains(r.stderr, "platforms") {
			t.Errorf("reported a later error:\n%s", r.stderr)
		}
	})
}
