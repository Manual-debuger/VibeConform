package gotooling

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestConfigsWaitForTheLock pins spec 0056's Desired Behavior 9:
// golangci-lint holds one lock per machine, and a second run that finds it
// held exits 3 with "parallel golangci-lint is running" unless the config
// sets run.allow-serial-runners. Agent hooks and parallel worktrees overlap
// routinely, so the generated config and the setup skill's Go reference
// both make a second run wait for the first.
func TestConfigsWaitForTheLock(t *testing.T) {
	reference, err := os.ReadFile(filepath.Join("..", "..", "setupskill", "skill", "reference", "lint", "go", ".golangci.yml"))
	if err != nil {
		t.Fatalf("reading the setup skill's Go reference: %v", err)
	}
	for name, content := range map[string][]byte{
		"embedded template":     golangciConfig,
		"setup skill reference": reference,
	} {
		var cfg struct {
			Run map[string]any `yaml:"run"`
		}
		if err := yaml.Unmarshal(content, &cfg); err != nil {
			t.Fatalf("%s: parsing YAML: %v", name, err)
		}
		if got, ok := cfg.Run["allow-serial-runners"].(bool); !ok || !got {
			t.Errorf("%s: run.allow-serial-runners = %v, want true", name, cfg.Run["allow-serial-runners"])
		}
	}
}
