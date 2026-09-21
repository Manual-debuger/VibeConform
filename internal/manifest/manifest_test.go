package manifest

import "testing"

func TestParse(t *testing.T) {
	m, err := Parse([]byte("standard: prod-go\nversion: v1\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if m.Standard != "prod-go" || m.Version != "v1" {
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

func TestNewMissingStandard(t *testing.T) {
	if _, err := New("", "v1"); err == nil {
		t.Fatal("expected error for missing standard, got nil")
	}
}

func TestNewMarshalParseRoundTrip(t *testing.T) {
	m, err := New("prod-go", "v1")
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := m.Marshal()
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if *got != *m {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, m)
	}
}
