package claude

import (
	"slices"
	"strings"
	"testing"
)

// TestCheckLocalAgreesWithTheGuards pins that audit and the three guards
// read policy.local.json alike (spec 0050 D7, D8): checkLocal calls a file
// broken exactly where the corpus expects the guards to deny everything.
func TestCheckLocalAgreesWithTheGuards(t *testing.T) {
	seen := 0
	for _, tc := range loadCorpus(t) {
		content := tc.localFile()
		if content == nil {
			continue
		}
		seen++
		want := strings.HasPrefix(tc.Name, "local broken:")
		if got := checkLocal(policy, content).Broken; (got != "") != want {
			t.Errorf("%s: Broken = %q, want broken %v", tc.Name, got, want)
		}
	}
	if seen == 0 {
		t.Fatal("no corpus case has a local file")
	}
}

func TestCheckLocalProblemsAndNotes(t *testing.T) {
	for _, c := range []struct {
		name, content string
		problems      []string
		notes         []string
	}{
		{
			name:    "exception and extra rule",
			content: `{"version": 1, "rules": [{"id": "tf", "field": "command", "pattern": "terraform destroy", "message": "Blocked."}], "exceptions": [{"rule": "git-clean-fd", "pattern": "git clean -fdx build/", "reason": "build output"}, {"rule": "tf", "pattern": "terraform destroy -target=x"}]}`,
			notes: []string{
				"extra rule tf: terraform destroy",
				"exception git-clean-fd: git clean -fdx build/ (build output)",
				"exception tf: terraform destroy -target=x",
			},
		},
		{
			name:     "unknown rule id",
			content:  `{"version": 1, "exceptions": [{"rule": "nope", "pattern": "x"}]}`,
			problems: []string{"exception for nope: no rule has that id, so it lifts nothing"},
			notes:    []string{"exception nope: x"},
		},
		{
			name:     "exception pattern does not compile",
			content:  `{"version": 1, "exceptions": [{"rule": "rm-rf", "pattern": "("}]}`,
			problems: []string{`exception for rm-rf: pattern "(" does not compile, so it lifts nothing`},
			notes:    []string{"exception rm-rf: ("},
		},
		{
			name:     "extra rule pattern does not compile",
			content:  `{"version": 1, "rules": [{"id": "x", "field": "command", "pattern": "(", "message": "Blocked."}]}`,
			problems: []string{`rule x: pattern "(" does not compile, so it denies every command`},
			notes:    []string{"extra rule x: ("},
		},
		{
			name:     "id taken by a managed rule",
			content:  `{"version": 1, "rules": [{"id": "rm-rf", "field": "command", "pattern": "rm", "message": "Blocked."}]}`,
			problems: []string{"rule id rm-rf is already taken; exceptions for it would be ambiguous"},
			notes:    []string{"extra rule rm-rf: rm"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := checkLocal(policy, []byte(c.content))
			if got.Broken != "" {
				t.Fatalf("broken: %s", got.Broken)
			}
			if !slices.Equal(got.Problems, c.problems) {
				t.Errorf("problems %q, want %q", got.Problems, c.problems)
			}
			if !slices.Equal(got.Notes, c.notes) {
				t.Errorf("notes %q, want %q", got.Notes, c.notes)
			}
		})
	}
}
