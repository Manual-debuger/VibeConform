package gotooling

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestTemplateMatchesLiveFile is the drift alarm spec 0005 predated: the
// embedded golangci config and this repository's own .golangci.yml are two
// copies of the same thing until the dogfood increment makes one the output
// of the other.
func TestTemplateMatchesLiveFile(t *testing.T) {
	live, err := os.ReadFile(filepath.Join("..", "..", "..", ".golangci.yml"))
	if err != nil {
		t.Fatalf("reading live .golangci.yml: %v", err)
	}
	if !bytes.Equal(live, golangciConfig) {
		t.Error("embedded template has drifted from the live .golangci.yml — " +
			"decide which one is right rather than re-seeding blindly")
	}
}

// TestTemplateIsLF — see the equivalent test in internal/module/ci/github for
// why CR bytes in an embedded template are a cross-platform defect.
func TestTemplateIsLF(t *testing.T) {
	if bytes.Contains(golangciConfig, []byte("\r")) {
		t.Error("golangci.yml contains CR bytes; the working copy it was embedded from is CRLF, " +
			"which makes this binary's output platform-dependent")
	}
}
