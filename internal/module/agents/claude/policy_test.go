package claude

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// unportable lists regular-expression syntax that Go RE2, Python re, and
// JavaScript RegExp do not all interpret the same way. policy.json is read
// by all three, so a pattern using any of these would enforce a different
// policy depending on the repository's language.
var unportable = []struct {
	syntax string
	why    string
}{
	{"(?=", "lookahead: unsupported by RE2"},
	{"(?!", "negative lookahead: unsupported by RE2"},
	{"(?<", "lookbehind or named group: syntax differs between engines"},
	{"(?P", "Python-style named group: unsupported by JavaScript"},
	{"(?>", "atomic group: unsupported by RE2 and JavaScript"},
	{"[[:", "POSIX class: unsupported by Python and JavaScript"},
	{`\A`, "start-of-text anchor: unsupported by JavaScript"},
	{`\z`, "end-of-text anchor: unsupported by Python and JavaScript"},
	{`\Z`, "end-of-text anchor: means different things in each engine"},
	{`\p{`, "Unicode class: needs the u flag in JavaScript"},
	{"*+", "possessive quantifier: unsupported by RE2 and JavaScript"},
	{"++", "possessive quantifier: unsupported by RE2 and JavaScript"},
	{"?+", "possessive quantifier: unsupported by RE2 and JavaScript"},
	{"}+", "possessive quantifier: unsupported by RE2 and JavaScript"},
}

// backreference matches \1 through \9, which RE2 rejects.
var backreference = regexp.MustCompile(`\\[1-9]`)

// inlineFlag matches (?x) style groups other than the non-capturing (?:.
var inlineFlag = regexp.MustCompile(`\(\?[a-zA-Z]`)

func TestPolicyPatternsInPortableSubset(t *testing.T) {
	for _, r := range policy.Rules {
		for name, pattern := range map[string]string{"pattern": r.Pattern, "raw": r.Raw} {
			if _, err := regexp.Compile(pattern); err != nil {
				t.Errorf("%s %s does not compile: %v", r.ID, name, err)
			}
			for _, u := range unportable {
				if strings.Contains(pattern, u.syntax) {
					t.Errorf("%s %s uses %q (%s)", r.ID, name, u.syntax, u.why)
				}
			}
			if backreference.MatchString(pattern) {
				t.Errorf("%s %s uses a backreference, which RE2 rejects", r.ID, name)
			}
			if inlineFlag.MatchString(pattern) {
				t.Errorf("%s %s uses an inline flag group, whose support differs between engines", r.ID, name)
			}
		}
	}
}

func TestPolicyRulesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range policy.Rules {
		if r.ID == "" || r.Pattern == "" || r.Raw == "" || r.Message == "" {
			t.Errorf("rule %+v has an empty field", r)
		}
		if seen[r.ID] {
			t.Errorf("duplicate rule id %q", r.ID)
		}
		seen[r.ID] = true
		if _, ok := policy.Tools[r.Field]; !ok {
			t.Errorf("%s inspects field %q, which no tool carries", r.ID, r.Field)
		}
	}
}

// TestPolicyRules pins the rule set: the seven rules ported from the bash
// hooks (spec 0021) plus git-discard-worktree (spec 0047). Adding or
// dropping a rule is a policy decision, so it has to change this list. Every
// message names the escape: the agent asks, the user runs the command with !
// (spec 0047, D5).
func TestPolicyRules(t *testing.T) {
	want := []string{
		"rm-rf", "remove-item-recurse-force", "git-reset-hard", "git-clean-fd",
		"git-push-force", "git-branch-force-delete", "git-discard-worktree", "secret-file",
		"guard-policy-file",
	}
	var got []string
	for _, r := range policy.Rules {
		got = append(got, r.ID)
		if !strings.HasPrefix(r.Message, "Blocked: ") || !strings.Contains(r.Message, "Ask the user to ") ||
			!strings.Contains(r.Message, "(in Claude Code: ! <command>") {
			t.Errorf("%s message %q does not say what was blocked and how the user runs it", r.ID, r.Message)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("rules %v, want %v", got, want)
	}
}

func TestRenderPolicyDeterministic(t *testing.T) {
	first, err := renderPolicy(policy)
	if err != nil {
		t.Fatalf("renderPolicy: %v", err)
	}
	second, err := renderPolicy(policy)
	if err != nil {
		t.Fatalf("renderPolicy: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("renderPolicy output differs across calls")
	}
	if bytes.Contains(first, []byte("\r")) {
		t.Error("policy.json contains CR bytes")
	}
	if !bytes.HasSuffix(first, []byte("}\n")) {
		t.Error("policy.json does not end with a single newline")
	}
	// json.Encoder's HTML escaping writes &, < and > as backslash-u escapes
	// (u0026, u003c, u003e). The patterns use & as a command separator (spec
	// 0047), so it must appear as itself. Some editing tools decode a backslash-u
	// sequence on write, which once left this check looking for & and < instead.
	if bytes.Contains(first, []byte(`\u0026`)) || bytes.Contains(first, []byte(`\u003c`)) || bytes.Contains(first, []byte(`\u003e`)) {
		t.Error("policy.json HTML-escapes its patterns; they must read as written")
	}

	var round Policy
	if err := json.Unmarshal(first, &round); err != nil {
		t.Fatalf("policy.json does not parse back: %v", err)
	}
	if len(round.Rules) != len(policy.Rules) || round.Rules[0].Pattern != policy.Rules[0].Pattern {
		t.Error("policy.json does not round-trip")
	}
}

// TestRenderPolicyIsPrettierStyle pins the one formatting choice that
// differs from json.Encoder: short string arrays on one line, the way
// Prettier writes them. prod-ts's fmt:check would otherwise fail on the
// policy.json it generates. The examples.yml TypeScript job runs Prettier
// over the real file; this pins the rule locally.
func TestRenderPolicyIsPrettierStyle(t *testing.T) {
	out, err := renderPolicy(policy)
	if err != nil {
		t.Fatalf("renderPolicy: %v", err)
	}
	for _, want := range []string{
		`    "command": ["Bash", "PowerShell"],`,
		`    "file_path": ["Edit", "MultiEdit", "Write"]`,
	} {
		if !bytes.Contains(out, []byte(want+"\n")) {
			t.Errorf("policy.json lacks the line %q", want)
		}
	}

	long := []byte("{\n  \"k\": [\n    \"" + strings.Repeat("a", 40) + "\",\n    \"" + strings.Repeat("b", 40) + "\"\n  ]\n}\n")
	if got := collapseStringArrays(long); !bytes.Equal(got, long) {
		t.Errorf("an array wider than %d columns was collapsed:\n%s", printWidth, got)
	}
}

func TestRenderPolicyRejectsUnsortedTools(t *testing.T) {
	p := policy
	p.Tools = map[Field][]string{FieldCommand: {"PowerShell", "Bash"}}
	if _, err := renderPolicy(p); err == nil {
		t.Error("renderPolicy accepted an unsorted tool list, which would make output order depend on source order")
	}
}
