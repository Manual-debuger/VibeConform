package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/guard"
)

// Unlock bounds, in minutes (spec 0053, "vibe unlock").
const (
	unlockDefaultMinutes = 15
	unlockMaxMinutes     = 240
)

func newUnlockCmd() *cobra.Command {
	var repoRoot string
	var minutes int
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Let the agent change its guardrail settings for a while (run it in your own terminal)",
		Long: "unlock writes the unlock marker (vibe-unlock in the common Git directory):\n" +
			"until it expires, the guard lets the agent edit .claude/settings.json and\n" +
			".claude/hooks/ (not the guard's policy files). The agent's next Stop locks\n" +
			"again; vibe-guard relock locks early. An agent's own call of vibe unlock is refused.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if minutes < 1 || minutes > unlockMaxMinutes {
				return fmt.Errorf("unlock: --minutes must be from 1 to %d, not %d", unlockMaxMinutes, minutes)
			}
			dir, err := gitCommonDir(repoRoot)
			if err != nil {
				return fmt.Errorf("unlock: %w", err)
			}
			expiry := time.Now().Add(time.Duration(minutes) * time.Minute).UTC().Truncate(time.Second)
			stamp := expiry.Format(time.RFC3339)
			marker := filepath.Join(dir, guard.MarkerFile)
			if err := os.WriteFile(marker, []byte(stamp+"\n"), 0o600); err != nil {
				return fmt.Errorf("unlock: %w", err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"unlocked until %s (%s)\n"+
					"The agent may now change .claude/settings.json and .claude/hooks/ (not the guard's policy files).\n"+
					"The agent's next Stop locks them again; to lock earlier, run: vibe-guard relock\n",
				stamp, marker)
			return err
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", ".", "repository whose agent guardrails to unlock")
	cmd.Flags().IntVar(&minutes, "minutes", unlockDefaultMinutes, fmt.Sprintf("how long the unlock lasts, 1 to %d", unlockMaxMinutes))
	return cmd
}

// gitCommonDir is git rev-parse --git-common-dir of repoRoot, as an
// absolute path.
func gitCommonDir(repoRoot string) (string, error) {
	c := exec.Command("git", "rev-parse", "--git-common-dir")
	c.Dir = repoRoot
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir in %s: %w: %s", repoRoot, err, strings.TrimSpace(stderr.String()))
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoRoot, dir)
	}
	return filepath.Abs(dir)
}
