package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalLefthookStaysConformant pins the audit half of spec 0036
// acceptance criterion 5: a project-owned lefthook.local.yml beside the
// managed lefthook.yml is not VibeConform's, so sync leaves it alone and
// audit stays conformant.
func TestLocalLefthookStaysConformant(t *testing.T) {
	for _, std := range []string{"prod-go", "prod-ts", "prod-py"} {
		t.Run(std, func(t *testing.T) {
			dir := t.TempDir()
			writeVibeYAML(t, dir, "standard: "+std+"\nversion: v1\n")
			mustSync(t, dir)

			local := []byte("post-merge:\n  commands:\n    deps:\n      run: task deps\n")
			path := filepath.Join(dir, "lefthook.local.yml")
			if err := os.WriteFile(path, local, 0o644); err != nil {
				t.Fatal(err)
			}
			mustConform(t, dir)
			out, err := runAuditIn(t, dir)
			if err != nil || !strings.Contains(out, "lefthook.yml: ok") {
				t.Errorf("audit: %v\n%s", err, out)
			}
			if strings.Contains(out, "lefthook.local.yml") {
				t.Errorf("audit lists lefthook.local.yml:\n%s", out)
			}

			sync := mustSync(t, dir)
			if strings.Contains(sync, "lefthook.local.yml") {
				t.Errorf("sync lists lefthook.local.yml:\n%s", sync)
			}
			if got, _ := os.ReadFile(path); string(got) != string(local) {
				t.Errorf("sync changed lefthook.local.yml")
			}
		})
	}
}
