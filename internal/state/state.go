// Package state reads .vibe/state.yaml, the machine-owned record of what
// VibeConform last applied to a repository. See
// docs/decisions/0002-desired-state.md and
// docs/specs/0006-reconcile-diff-v1.md. vibe sync writes it (see
// docs/specs/0008-vibe-sync-v1.md); every other command only reads it.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Manual-debuger/VibeConform/internal/atomicfile"
)

const (
	// stateDirName is the repository-relative directory holding
	// VibeConform's machine-owned files.
	stateDirName = ".vibe"
	// stateFilePath is the repository-relative location of the state file.
	stateFilePath = ".vibe/state.yaml"
)

// ResourceState is what was last recorded for one resolved resource.
type ResourceState struct {
	// SHA256 is the hex-encoded content hash last applied for this path.
	SHA256 string `yaml:"sha256"`
}

// State is the parsed form of .vibe/state.yaml, keyed by repository-relative
// resource path.
type State struct {
	Resources map[string]ResourceState `yaml:"resources"`
}

// Load reads .vibe/state.yaml from repoRoot. A missing file is not an
// error: it returns an empty State, since no vibe sync has ever run.
func Load(repoRoot string) (*State, error) {
	path := filepath.Join(repoRoot, stateFilePath)
	data, err := os.ReadFile(path) // #nosec G304 -- repoRoot is an operator-supplied CLI flag, same trust boundary as init.go's WriteFile target
	if errors.Is(err, os.ErrNotExist) {
		return &State{Resources: map[string]ResourceState{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if s.Resources == nil {
		s.Resources = map[string]ResourceState{}
	}
	return &s, nil
}

// Save writes s to .vibe/state.yaml under repoRoot, creating the .vibe
// directory if it does not exist. The write is atomic: an interrupted run
// leaves the previous state file intact rather than a truncated one, since
// a half-written state file would corrupt every subsequent reconciliation
// decision.
func Save(repoRoot string, s *State) error {
	if err := os.MkdirAll(filepath.Join(repoRoot, stateDirName), 0o750); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if err := atomicfile.Write(filepath.Join(repoRoot, stateFilePath), data, 0o600); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
