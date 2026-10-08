package gitlabmono

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/module/conformance"
)

var update = flag.Bool("update", false, "rewrite the golden files from the current templates")

// TestGolden pins the whole GitLab output for examples/monorepo's components.
// No repository in this one commits a gitlab copy, so without it a template
// change would reach adopters with no reviewable diff: a change to the
// output must show up as a change to testdata/ (spec 0044). Rewrite with
//
//	go test ./internal/module/ci/gitlabmono -run TestGolden -update
func TestGolden(t *testing.T) {
	pipelineRs, err := New().Resolve(context.Background(), gitlabContext(components))
	if err != nil {
		t.Fatal(err)
	}
	conformanceRs, err := conformance.New().Resolve(context.Background(), gitlabContext(components))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]byte{}
	for _, r := range append(pipelineRs, conformanceRs...) {
		switch r.Path {
		case PipelinePath, conformance.GitLabPath:
			got[r.Path] = r.Content
		}
	}
	for _, path := range []string{PipelinePath, conformance.GitLabPath} {
		golden := filepath.Join("testdata", "monorepo"+path+".golden")
		if *update {
			if err := os.WriteFile(golden, got[path], 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("%v (run with -update to create it)", err)
		}
		if !bytes.Equal(got[path], want) {
			t.Errorf("%s differs from %s; review the change, then run with -update", path, golden)
		}
	}
}
