package manifest

import (
	"strings"
	"testing"
)

func TestPolicyAbsentByDefault(t *testing.T) {
	m, err := Parse([]byte("standard: prod-go\nversion: v1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Policy != nil || m.Policy.Get(PolicyLineEndings) != nil {
		t.Errorf("policy %+v, want absent", m.Policy)
	}
}

func TestPolicyLineEndings(t *testing.T) {
	m, err := Parse([]byte("standard: prod-go\nversion: v1\npolicy:\n  line_endings: lf\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Policy.Get(PolicyLineEndings); got == nil || *got != "lf" {
		t.Errorf("line_endings = %v, want lf", got)
	}
}

// TestPolicyIsStrict: a misspelled key must fail, not select nothing.
func TestPolicyIsStrict(t *testing.T) {
	for _, body := range []string{
		"policy:\n  line_ending: lf\n",
		"policies:\n  line_endings: lf\n",
		"policy:\n  line_endings: [lf]\n",
	} {
		if _, err := Parse([]byte("standard: prod-go\nversion: v1\n" + body)); err == nil {
			t.Errorf("%q: accepted", body)
		} else if !strings.Contains(err.Error(), "parse manifest") {
			t.Errorf("%q: error %v", body, err)
		}
	}
}
