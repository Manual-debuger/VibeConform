package contract_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// TestRepositoryConfigIsVersion2: this repository's own vibe.yaml is the
// contract configuration (spec 0059, Desired Behavior 1), and the
// contract's parser reads it, so `vibe audit` with no --config audits it.
func TestRepositoryConfigIsVersion2(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "vibe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := contract.DetectSchema("vibe.yaml", data)
	if err != nil {
		t.Fatal(err)
	}
	if schema != contract.SchemaV2 {
		t.Fatalf("vibe.yaml is schema %v, want version 2 (a top-level contract key)", schema)
	}
	if _, err := os.Stat(filepath.Join(root, "vibe.v2.yaml")); err == nil {
		t.Error("vibe.v2.yaml still exists beside vibe.yaml")
	}
	repo, err := contract.OpenRepo(context.Background(), root)
	if err != nil {
		t.Skipf("cannot list this checkout's files: %v", err)
	}
	if _, err := contract.Parse("vibe.yaml", data, repo); err != nil {
		t.Errorf("vibe.yaml does not parse: %v", err)
	}
}
