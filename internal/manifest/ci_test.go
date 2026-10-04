package manifest

import (
	"strings"
	"testing"
)

// TestParseCI pins spec 0038 acceptance criterion 2's decoding half: the
// ci: map takes provider, strictly; which values a standard offers is the
// standard's to say.
func TestParseCI(t *testing.T) {
	m, err := Parse([]byte("standard: prod-mono\nversion: v1\nci:\n  provider: gitlab\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Scalar(ScalarKey{MapCI, CIProvider}); got == nil || *got != "gitlab" {
		t.Errorf("ci.provider = %v", got)
	}
	m, err = Parse([]byte("standard: prod-mono\nversion: v1\n"))
	if err != nil || m.Scalar(ScalarKey{MapCI, CIProvider}) != nil {
		t.Errorf("absent ci: = %v, %v", m.Scalar(ScalarKey{MapCI, CIProvider}), err)
	}
	if _, err := Parse([]byte("standard: prod-mono\nversion: v1\nci:\n  providr: gitlab\n")); err == nil || !strings.Contains(err.Error(), "providr") {
		t.Errorf("unknown key under ci: error %v", err)
	}
}
