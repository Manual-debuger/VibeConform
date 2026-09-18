package manifest

import "testing"

func TestParse(t *testing.T) {
	m, err := Parse([]byte("standard: production\nversion: v1\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if m.Standard != "production" || m.Version != "v1" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
}

func TestParseMissingStandard(t *testing.T) {
	if _, err := Parse([]byte("version: v1\n")); err == nil {
		t.Fatal("expected error for missing standard, got nil")
	}
}

func TestParseInvalidYAML(t *testing.T) {
	if _, err := Parse([]byte("standard: [unterminated\n")); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
