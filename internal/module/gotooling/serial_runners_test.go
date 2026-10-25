package gotooling

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestConfigsWaitForTheLock pins spec 0056's Desired Behavior 9:
// golangci-lint holds one lock per machine, and a second run that finds it
// held exits 3 with "parallel golangci-lint is running" unless the config
// sets run.allow-serial-runners. Agent hooks and parallel worktrees overlap
// routinely, so the generated config makes a second run wait for the
// first. The setup skill's Go reference is tested in internal/setupskill
// (spec 0059).
func TestConfigsWaitForTheLock(t *testing.T) {
	for name, content := range map[string][]byte{
		"embedded template": golangciConfig,
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
