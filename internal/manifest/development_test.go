package manifest

import (
	"strings"
	"testing"
)

func TestDevelopmentAbsentByDefault(t *testing.T) {
	m, err := Parse([]byte("standard: prod-go\nversion: v1\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range ScalarKeys {
		if v := m.Scalar(k); v != nil {
			t.Errorf("%s.%s = %q, want absent", k.Map, k.Key, *v)
		}
	}
}

func TestDevelopmentKeys(t *testing.T) {
	m, err := Parse([]byte("standard: prod-go\nversion: v1\ndevelopment:\n  workflow: always-sdd\n  docs_layout: standard\n"))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{DevelopmentWorkflow: "always-sdd", DevelopmentDocsLayout: "standard"} {
		if got := m.Scalar(ScalarKey{MapDevelopment, key}); got == nil || *got != want {
			t.Errorf("development.%s = %v, want %s", key, got, want)
		}
	}
}

// TestDevelopmentIsStrict: a misspelled key, or two workflows at once,
// must fail rather than select nothing.
func TestDevelopmentIsStrict(t *testing.T) {
	for _, body := range []string{
		"development:\n  workflows: direct\n",
		"develop:\n  workflow: direct\n",
		"development:\n  workflow: [direct, always-sdd]\n",
	} {
		if _, err := Parse([]byte("standard: prod-go\nversion: v1\n" + body)); err == nil {
			t.Errorf("%q: accepted", body)
		} else if !strings.Contains(err.Error(), "parse manifest") {
			t.Errorf("%q: error %v", body, err)
		}
	}
}
