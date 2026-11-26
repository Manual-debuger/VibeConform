package setupskill_test

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/setupskill"
)

// updateRenderGoldens rewrites the rendered-skill goldens from the
// current Render:
//
//	go test ./internal/setupskill -run TestRenderGolden -update
//
// Review the diff before committing it. A golden changes whenever a
// skill file or a substitution changes.
var updateRenderGoldens = flag.Bool("update", false, "rewrite the rendered-skill goldens; review the diff, then commit it")

// TestRenderGolden: Render gives, byte for byte, the skill that vibe
// init --skill wrote before spec 0073 named the release page as a
// placeholder, for a release (the tag page) and for an unstamped build
// (latest, the releases page). The goldens were recorded from main at
// e280d94, before the change. Each line is a file's SHA-256 and its
// path, in the form sha256sum writes, so the golden does not copy the
// skill.
func TestRenderGolden(t *testing.T) {
	for _, tc := range []struct{ name, vibeVersion string }{
		{"tag", "v0.12.0-alpha.1"},
		{"latest", "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, err := setupskill.Render(tc.vibeVersion)
			if err != nil {
				t.Fatal(err)
			}
			got := renderDigest(files)
			golden := filepath.Join("testdata", "render", tc.name+".golden")
			if want := readGolden(t, golden, got); !bytes.Equal(got, want) {
				t.Errorf("Render(%q) differs from %s:\ngot\n%s\nwant\n%s", tc.vibeVersion, golden, got, want)
			}
		})
	}
}

// readGolden returns the golden file's content, after writing got to it
// when -update is given.
func readGolden(t *testing.T, golden string, got []byte) []byte {
	t.Helper()
	if *updateRenderGoldens {
		if err := os.MkdirAll(filepath.Dir(golden), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	return want
}

// renderDigest is one line per file, "<sha256>  <path>", sorted by path.
func renderDigest(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var b bytes.Buffer
	for _, name := range names {
		fmt.Fprintf(&b, "%x  %s\n", sha256.Sum256(files[name]), name)
	}
	return b.Bytes()
}
