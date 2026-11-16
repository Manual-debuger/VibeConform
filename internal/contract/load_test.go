package contract_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

// loadFixture writes files into a new directory and returns it with an
// opener that lists exactly those files, so the fixture needs no git.
func loadFixture(t *testing.T, files map[string]string) (string, contract.RepoOpener) {
	t.Helper()
	dir := t.TempDir()
	var names []string
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	open := func(_ context.Context, root string) (*contract.Repo, error) {
		return contract.NewRepo(root, names), nil
	}
	return dir, open
}

const (
	loadGoMod  = "module fixture\n\ngo 1.22\n"
	loadValid  = "contract: 1.2.0\nplatforms: [linux]\nagents: []\n"
	loadV1     = "standard: prod-go\nversion: v1\n"
	loadBoth   = "contract: 1.2.0\nstandard: prod-go\n"
	loadNone   = "platforms: [linux]\n"
	loadBadV2  = "contract: 1.2.0\nplatforms: [bsd]\nagents: []\n"
	loadMarker = "vibe.yaml"
)

// The outcomes of contract.Load (spec 0065, "Testing Decisions"), one
// test each.

func TestLoadWithoutVibeYAMLIsErrNoConfig(t *testing.T) {
	t.Parallel()
	dir, open := loadFixture(t, map[string]string{"go.mod": loadGoMod})
	_, _, err := contract.Load(context.Background(), dir, "", open)
	if !errors.Is(err, contract.ErrNoConfig) {
		t.Fatalf("err = %v, want ErrNoConfig", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("error does not name the root %s: %v", dir, err)
	}
}

func TestLoadMissingConfigFileIsAReadError(t *testing.T) {
	t.Parallel()
	dir, open := loadFixture(t, map[string]string{"go.mod": loadGoMod, "vibe.yaml": loadValid})
	path := filepath.Join(dir, "absent.yaml")
	_, _, err := contract.Load(context.Background(), dir, path, open)
	if err == nil {
		t.Fatal("no error for a missing --config file")
	}
	if errors.Is(err, contract.ErrNoConfig) {
		t.Errorf("a missing --config file is ErrNoConfig: %v", err)
	}
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "absent.yaml") {
		t.Errorf("err = %v, want a read error naming absent.yaml", err)
	}
}

func TestLoadVersion1IsErrConfigV1NamingTheFile(t *testing.T) {
	t.Parallel()
	dir, open := loadFixture(t, map[string]string{"go.mod": loadGoMod, "old.yaml": loadV1})
	path := filepath.Join(dir, "old.yaml")
	_, _, err := contract.Load(context.Background(), dir, path, open)
	if !errors.Is(err, contract.ErrConfigV1) {
		t.Fatalf("err = %v, want ErrConfigV1", err)
	}
	if !strings.HasPrefix(err.Error(), path+" ") {
		t.Errorf("error does not start with the file %s: %v", path, err)
	}
}

func TestLoadInvalidFileIsAConfigError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, c := range []struct{ name, content, path string }{
		{"both keys", loadBoth, ""},
		{"neither key", loadNone, ""},
		{"an invalid version 2 file", loadBadV2, "platforms[0]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir, open := loadFixture(t, map[string]string{"go.mod": loadGoMod, "vibe.yaml": c.content})
			_, _, err := contract.Load(ctx, dir, "", open)
			var ce *contract.ConfigError
			if !errors.As(err, &ce) {
				t.Fatalf("err = %v, want a *ConfigError", err)
			}
			if ce.File != loadMarker || ce.Path != c.path {
				t.Errorf("ConfigError names %q at %q, want %q at %q", ce.File, ce.Path, loadMarker, c.path)
			}
			if errors.Is(err, contract.ErrNoConfig) || errors.Is(err, contract.ErrConfigV1) {
				t.Errorf("a ConfigError is a sentinel too: %v", err)
			}
		})
	}
}

func TestLoadValidFileGivesTheConfigurationAndItsRepository(t *testing.T) {
	t.Parallel()
	dir, open := loadFixture(t, map[string]string{"go.mod": loadGoMod, "vibe.yaml": loadValid})
	var opened *contract.Repo
	cfg, repo, err := contract.Load(context.Background(), dir, "", func(ctx context.Context, root string) (*contract.Repo, error) {
		r, err := open(ctx, root)
		opened = r
		return r, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Contract.Declared != "1.2.0" {
		t.Errorf("declared contract = %q, want 1.2.0", cfg.Contract.Declared)
	}
	if repo == nil || repo != opened {
		t.Errorf("repo = %p, want the opener's %p", repo, opened)
	}
}

func TestLoadReturnsTheOpenersError(t *testing.T) {
	t.Parallel()
	dir, _ := loadFixture(t, map[string]string{"go.mod": loadGoMod, "vibe.yaml": loadValid})
	boom := errors.New("boom")
	_, _, err := contract.Load(context.Background(), dir, "", func(context.Context, string) (*contract.Repo, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the opener's", err)
	}
}

// ReadConfig is the read-and-detect step on its own, for vibe init: a
// version 2 file comes back with its label and content.
func TestReadConfigReturnsAVersion2File(t *testing.T) {
	t.Parallel()
	dir, _ := loadFixture(t, map[string]string{"vibe.yaml": loadValid})
	file, data, err := contract.ReadConfig(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if file != loadMarker || string(data) != loadValid {
		t.Errorf("ReadConfig = %q, %q; want %q and the file's content", file, data, loadMarker)
	}
}
