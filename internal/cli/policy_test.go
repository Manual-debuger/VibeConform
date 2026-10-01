package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/state"
)

const (
	goLF = "standard: prod-go\nversion: v1\npolicy:\n  line_endings: lf\n"

	// policyGitattributes is the whole file the policy writes when there is
	// none, byte for byte.
	policyGitattributes = "# vibeconform:begin line-endings\n" +
		"# Managed by VibeConform: policy.line_endings in vibe.yaml.\n" +
		"* text=auto eol=lf\n" +
		"# vibeconform:end line-endings\n"

	// policyHash is the sha256 state records for the section's content. It
	// is a constant, and CI runs this test on Ubuntu and on Windows: the
	// same bytes and the same hash on both is spec 0029 §6's claim.
	policyHash = "eac2d782a61f1a91384f56fe144d1fdc5798ea6a6ebe261e5db48b3f924ff838"
)

var policyKey = state.SectionKey{Path: ".gitattributes", ID: "line-endings"}

func writeGitattributes(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLineEndingPolicyIsOptIn: without policy:, nothing touches
// .gitattributes.
func TestLineEndingPolicyIsOptIn(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goDefaults)
	if out := runDiffIn(t, dir); strings.Contains(out, ".gitattributes") {
		t.Errorf("diff mentions .gitattributes:\n%s", out)
	}
	mustSync(t, dir)
	if exists(t, dir, ".gitattributes") {
		t.Error("sync wrote .gitattributes")
	}
}

func TestLineEndingPolicyGolden(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goLF)
	out := mustSync(t, dir)
	if !strings.Contains(out, ".gitattributes (section line-endings): created") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, ".gitattributes"); got != policyGitattributes {
		t.Errorf(".gitattributes = %q, want %q", got, policyGitattributes)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Sections[policyKey]; got != (state.SectionState{SHA256: policyHash, Created: true}) {
		t.Errorf("state %+v, want sha256 %s, created", got, policyHash)
	}
	mustConform(t, dir)
}

func TestLineEndingPolicyKeepsUserRules(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goLF)
	user := "*.png binary\r\n*.bat eol=crlf\r\n"
	writeGitattributes(t, dir, user)
	mustSync(t, dir)
	if got := readFile(t, dir, ".gitattributes"); got != policyGitattributes+"\n"+user {
		t.Errorf(".gitattributes = %q", got)
	}
	mustConform(t, dir)

	// Deselecting removes the section and nothing else.
	writeVibeYAML(t, dir, goDefaults)
	if out := mustSync(t, dir); !strings.Contains(out, ".gitattributes (section line-endings): removed (policy.line_endings deselected)") {
		t.Errorf("sync:\n%s", out)
	}
	if got := readFile(t, dir, ".gitattributes"); got != user {
		t.Errorf(".gitattributes = %q, want the user's rules alone", got)
	}
	mustConform(t, dir)
}

func TestLineEndingPolicyDeselectDeletesCreatedFile(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goLF)
	mustSync(t, dir)
	writeVibeYAML(t, dir, goDefaults)
	mustSync(t, dir)
	if exists(t, dir, ".gitattributes") {
		t.Error(".gitattributes survived, though VibeConform created it and nothing else is in it")
	}
}

// TestLineEndingPolicyGlobalContradiction: a later rule on every path
// undoes the policy, so it is a conflict; a narrower one is not.
func TestLineEndingPolicyGlobalContradiction(t *testing.T) {
	dir := t.TempDir()
	writeVibeYAML(t, dir, goLF)
	mustSync(t, dir)

	contradicted := policyGitattributes + "\n* eol=crlf\n"
	writeGitattributes(t, dir, contradicted)
	out, err := runAuditIn(t, dir)
	wantExit(t, err, 2, out)
	if !strings.Contains(out, `.gitattributes (section line-endings): conflict: line 6: "* eol=crlf" overrides policy.line_endings (eol=crlf) for every path`) {
		t.Errorf("audit:\n%s", out)
	}
	if out := runDiffIn(t, dir); !strings.Contains(out, "conflict: line 6:") {
		t.Errorf("diff:\n%s", out)
	}
	out, err = runSyncIn(t, dir)
	if err == nil || !strings.Contains(out, "conflict: line 6:") {
		t.Errorf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, dir, ".gitattributes"); got != contradicted {
		t.Errorf("sync rewrote a contradicted file: %q", got)
	}

	writeGitattributes(t, dir, policyGitattributes+"\n*.bat eol=crlf\n")
	mustConform(t, dir)
}
