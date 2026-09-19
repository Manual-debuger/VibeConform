package github

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

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
