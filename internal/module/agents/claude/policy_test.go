package claude

import (
	"bytes"
	"encoding/json"
	"regexp"
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

// TestPolicyIsFaithfulPort guards spec 0021's "no new patterns" rule from
// the other direction: every deny message the old bash hooks could print
// must still come from exactly one rule, so porting did not drop one.
func TestPolicyIsFaithfulPort(t *testing.T) {
	oldMessages := []string{
		"Blocked: rm -rf (or equivalent) is a destructive operation. Ask the user to run it themselves if intended.",
		"Blocked: Remove-Item -Recurse -Force is a destructive operation. Ask the user to run it themselves if intended.",
		"Blocked: git reset --hard discards uncommitted work. Ask the user to run it themselves if intended.",
		"Blocked: git clean -fd (or equivalent) permanently deletes untracked files. Ask the user to run it themselves if intended.",
		"Blocked: git push --force can overwrite remote history. Ask the user to run it themselves if intended.",
		"Blocked: git branch -D force-deletes a branch. Ask the user to run it themselves if intended.",
		"Blocked: this looks like a secret/credential file. Ask the user to edit it themselves.",
	}
	if len(policy.Rules) != len(oldMessages) {
		t.Errorf("policy has %d rules, the old hooks had %d", len(policy.Rules), len(oldMessages))
	}
	for _, msg := range oldMessages {
		n := 0
		for _, r := range policy.Rules {
			if r.Message == msg {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%d rules print %q, want exactly 1", n, msg)
		}
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
	if bytes.Contains(first, []byte(`<`)) || bytes.Contains(first, []byte(`&`)) {
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
