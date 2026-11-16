package contract

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ConfigFile is the configuration read at the repository root when no
// path names another file.
const ConfigFile = "vibe.yaml"

// The load errors a caller tells apart with errors.Is (spec 0065, "The
// contract loads its configuration"). Their text is the error's, not the
// sentinel's: the caller adds what the user should do.
var (
	// ErrNoConfig: no path was given and the root has no vibe.yaml. A
	// missing file named by a path is a read error instead.
	ErrNoConfig = errors.New("no configuration")
	// ErrConfigV1: the file is version 1 (a top-level standard key), the
	// removed template-sync system's file. The error names the file.
	ErrConfigV1 = errors.New("configuration is version 1")
)

// loadError is a load error with its own text that errors.Is matches to
// its sentinel.
type loadError struct {
	msg      string
	sentinel error
}

func (e *loadError) Error() string { return e.msg }
func (e *loadError) Unwrap() error { return e.sentinel }

// RepoOpener lists the tracked files of the repository at root.
// OpenRepo is the real one; a test hands in a fixture's file list.
type RepoOpener func(ctx context.Context, root string) (*Repo, error)

// ReadConfig reads the configuration at configPath, or vibe.yaml at root
// when configPath is empty, and returns its label and content when it is
// version 2. The label is configPath as given, or ConfigFile. It is the
// one place where version 1 is told from version 2.
//
// A missing vibe.yaml is ErrNoConfig, a version 1 file is ErrConfigV1, a
// file that is neither version (or both) is a *ConfigError, and a file
// that cannot be read is the read error wrapped, an *fs.PathError
// inside.
func ReadConfig(root, configPath string) (file string, data []byte, err error) {
	path, file := configPath, configPath
	if path == "" {
		path, file = filepath.Join(root, ConfigFile), ConfigFile
	}
	data, err = os.ReadFile(path) // #nosec G304 -- operator-supplied configuration path
	if err != nil {
		if configPath == "" && errors.Is(err, fs.ErrNotExist) {
			return "", nil, &loadError{msg: fmt.Sprintf("no %s in %s", ConfigFile, root), sentinel: ErrNoConfig}
		}
		return "", nil, fmt.Errorf("cannot read the configuration: %w", err)
	}
	schema, err := DetectSchema(file, data)
	if err != nil {
		return "", nil, err
	}
	if schema == SchemaV1 {
		return "", nil, &loadError{
			msg:      file + " is version 1 (a top-level standard key), which this vibe no longer reads",
			sentinel: ErrConfigV1,
		}
	}
	return file, data, nil
}

// Load reads the configuration as ReadConfig does, opens the repository
// at root with open (nil means OpenRepo) and parses the configuration
// against it. Its errors are ReadConfig's, the opener's, and Parse's
// *ConfigError.
func Load(ctx context.Context, root, configPath string, open RepoOpener) (*Config, *Repo, error) {
	file, data, err := ReadConfig(root, configPath)
	if err != nil {
		return nil, nil, err
	}
	if open == nil {
		open = OpenRepo
	}
	repo, err := open(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := Parse(file, data, repo)
	if err != nil {
		return nil, nil, err
	}
	return cfg, repo, nil
}
