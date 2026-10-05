package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestReleaseMarksPrereleases pins ADR 0007: a -alpha.N, -beta.N or -rc.N
// tag must become a GitHub "Pre-release". GoReleaser marks one only with
// release.prerelease: auto. Without it, v0.6.0-alpha.1 and v0.7.0-alpha.1
// were drafted as ordinary releases, so publishing one would have made a
// preview the repository's "Latest release".
func TestReleaseMarksPrereleases(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Release struct {
			Draft      bool   `yaml:"draft"`
			Prerelease string `yaml:"prerelease"`
		} `yaml:"release"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Release.Prerelease != "auto" {
		t.Errorf("release.prerelease = %q, want auto (ADR 0007)", cfg.Release.Prerelease)
	}
	if !cfg.Release.Draft {
		t.Error("release.draft is off; a person must publish each release (ADR 0007)")
	}
}
