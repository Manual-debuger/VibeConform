package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type releaseConfig struct {
	Builds []struct {
		ID      string   `yaml:"id"`
		Main    string   `yaml:"main"`
		Binary  string   `yaml:"binary"`
		Env     []string `yaml:"env"`
		Goos    []string `yaml:"goos"`
		Goarch  []string `yaml:"goarch"`
		Ldflags []string `yaml:"ldflags"`
	} `yaml:"builds"`
	Archives []struct {
		ID           string   `yaml:"id"`
		IDs          []string `yaml:"ids"`
		NameTemplate string   `yaml:"name_template"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
	Release struct {
		Draft      bool   `yaml:"draft"`
		Prerelease string `yaml:"prerelease"`
	} `yaml:"release"`
}

func readReleaseConfig(t *testing.T) releaseConfig {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg releaseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestReleaseMarksPrereleases pins ADR 0007 as amended by spec 0054: a tag
// push publishes the release without a draft step, and a -alpha.N, -beta.N
// or -rc.N tag becomes a GitHub "Pre-release", which keeps a preview out of
// "Latest release". GoReleaser marks one only with release.prerelease: auto.
func TestReleaseMarksPrereleases(t *testing.T) {
	cfg := readReleaseConfig(t)
	if cfg.Release.Prerelease != "auto" {
		t.Errorf("release.prerelease = %q, want auto (ADR 0007)", cfg.Release.Prerelease)
	}
	if cfg.Release.Draft {
		t.Error("release.draft is on; a tag push must publish the release (spec 0054)")
	}
}

// TestReleaseBuildsBothTools pins spec 0054's "Two builds, one release":
// vibe and vibe-guard, each for linux, darwin and windows on amd64 and
// arm64, static, in an archive of its own named after the binary.
//
// The version is stamped from the tag, not from GoReleaser's .Version,
// which drops the leading v. A binary that calls itself 0.12.0-alpha.1
// instead of v0.12.0-alpha.1 records a vibe_version that task audit's
// fallback cannot fetch, and does not print the tag it was released as.
func TestReleaseBuildsBothTools(t *testing.T) {
	cfg := readReleaseConfig(t)
	want := map[string]string{"vibe": "./cmd/vibe", "vibe-guard": "./cmd/vibe-guard"}
	if len(cfg.Builds) != len(want) {
		t.Fatalf("%d builds, want %d (vibe and vibe-guard)", len(cfg.Builds), len(want))
	}
	for _, b := range cfg.Builds {
		main, ok := want[b.ID]
		if !ok {
			t.Errorf("unexpected build %q", b.ID)
			continue
		}
		if b.Main != main || b.Binary != b.ID {
			t.Errorf("build %s: main %q binary %q, want main %q binary %q", b.ID, b.Main, b.Binary, main, b.ID)
		}
		if !slices.Contains(b.Env, "CGO_ENABLED=0") {
			t.Errorf("build %s is not built with CGO_ENABLED=0", b.ID)
		}
		for _, goos := range []string{"linux", "darwin", "windows"} {
			if !slices.Contains(b.Goos, goos) {
				t.Errorf("build %s has no %s target", b.ID, goos)
			}
		}
		for _, arch := range []string{"amd64", "arm64"} {
			if !slices.Contains(b.Goarch, arch) {
				t.Errorf("build %s has no %s target", b.ID, arch)
			}
		}
		if len(b.Ldflags) != 1 || !strings.Contains(b.Ldflags[0], "-X main.version={{ .Tag }}") {
			t.Errorf("build %s ldflags = %q, want the version stamped from {{ .Tag }}", b.ID, b.Ldflags)
		}
	}

	archives := map[string]string{}
	for _, a := range cfg.Archives {
		if len(a.IDs) != 1 {
			t.Errorf("archive %s packs builds %q, want exactly one", a.ID, a.IDs)
			continue
		}
		archives[a.IDs[0]] = strings.TrimSpace(a.NameTemplate)
	}
	for id := range want {
		if got, wantName := archives[id], id+"_{{ .Version }}_{{ .Os }}_{{ .Arch }}"; got != wantName {
			t.Errorf("archive of %s is named %q, want %q", id, got, wantName)
		}
	}
	if cfg.Checksum.NameTemplate != "checksums.txt" {
		t.Errorf("checksum file = %q, want checksums.txt", cfg.Checksum.NameTemplate)
	}
}
