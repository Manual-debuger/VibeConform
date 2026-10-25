package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Manual-debuger/VibeConform/internal/contract"
)

func TestMain(m *testing.M) {
	// The full audit's R-01 shim is a copy of the running executable,
	// which here is this test binary.
	if code, ok := contract.RunAsShim(); ok {
		os.Exit(code)
	}
	// A git hook running the suite, such as pre-push in a linked worktree,
	// exports GIT_DIR. Every git the tests start would then use this
	// repository instead of the one in its temp directory.
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		_ = os.Unsetenv(v)
	}
	os.Exit(checkTestLog(m.Run()))
}

func writeVibeYAML(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "vibe.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(t *testing.T, dir, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}
