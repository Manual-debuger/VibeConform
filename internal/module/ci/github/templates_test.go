package github

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// embeddedTemplates is every template this module compiles into the binary.
var embeddedTemplates = map[string][]byte{
	".github/workflows/ci.yml":         ciWorkflow,
	".github/dependabot.yml":           dependabotConfig,
	".github/pull_request_template.md": pullRequestTemplate,
}

// TestTemplatesAreLF guards a defect that is invisible without it: go:embed
// reads the working copy at build time, so a CRLF checkout on Windows
// produces a different binary than an LF checkout on Linux — different
// embedded bytes, different content hashes, and a repository that reports
// drift depending on which machine built the tool that syncs it.
//
// This repository pins "* text=auto eol=lf" in .gitattributes, so a correct
// checkout never contains CR. Asserting it here turns a silent, per-machine
// divergence into a failing test.
func TestTemplatesAreLF(t *testing.T) {
	for path, content := range embeddedTemplates {
		if bytes.Contains(content, []byte("\r")) {
			t.Errorf("%s contains CR bytes; the working copy it was embedded from is CRLF, "+
				"which makes this binary's output platform-dependent", path)
		}
	}
}

// TestTemplatesMatchLiveFiles is the drift alarm for the window between this
// module landing and docs/specs/0013-dogfood-self-management.md, during which
// every managed file exists twice: live in this repository, and embedded
// here. Once this repository syncs itself the duplication disappears, but
// until then nothing else would notice the two copies diverging.
func TestTemplatesMatchLiveFiles(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..", "..")

	cases := []struct {
		live     string
		embedded []byte
	}{
		{filepath.Join(".github", "workflows", "ci.yml"), ciWorkflow},
		{filepath.Join(".github", "dependabot.yml"), dependabotConfig},
		{filepath.Join(".github", "pull_request_template.md"), pullRequestTemplate},
	}

	for _, tc := range cases {
		t.Run(tc.live, func(t *testing.T) {
			live, err := os.ReadFile(filepath.Join(repoRoot, tc.live))
			if err != nil {
				t.Fatalf("reading live file: %v", err)
			}
			if !bytes.Equal(live, tc.embedded) {
				t.Errorf("embedded template has drifted from %s\n"+
					"re-seed the template from the live file, or sync the live file from the template — "+
					"but decide which one is right rather than assuming", tc.live)
			}
		})
	}
}
