package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Manual-debuger/VibeConform/internal/guardrule"
)

// Unlock bounds, in minutes (spec 0053, "vibe unlock").
const (
	unlockDefaultMinutes = 15
	unlockMaxMinutes     = 240
)

func newUnlockCmd() *cobra.Command {
	var projectRoot string
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
			dir, err := unlockDir(projectRoot)
			if err != nil {
				return fmt.Errorf("unlock: %w", err)
			}
			expiry := time.Now().Add(time.Duration(minutes) * time.Minute).UTC().Truncate(time.Second)
			stamp := expiry.Format(time.RFC3339)
			marker := filepath.Join(dir, guardrule.UnlockMarkerFile)
			if err := os.WriteFile(marker, guardrule.FormatUnlockMarker(expiry), 0o600); err != nil {
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
	cmd.Flags().StringVar(&projectRoot, "repo-root", ".", "project root whose agent guardrails to unlock")
	cmd.Flags().IntVar(&minutes, "minutes", unlockDefaultMinutes, fmt.Sprintf("how long the unlock lasts, 1 to %d", unlockMaxMinutes))
	return cmd
}

// unlockDir is the common Git directory of the project root projectRoot,
// where the unlock marker goes. It is found as vibe-guard finds it (spec
// 0069): by reading .git in projectRoot or the nearest directory above
// it, the Git top, never by running git, so GIT_DIR and GIT_COMMON_DIR
// are not read.
func unlockDir(projectRoot string) (string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	repo := guardrule.FindRepo(root, root)
	switch {
	case repo.CommonDir != "":
		return repo.CommonDir, nil
	case repo.GitTop != "":
		return "", fmt.Errorf("no Git directory found for %s: %s could not be read as vibe-guard reads it "+
			"(a .git file needs a \"gitdir: <path>\" line, and a commondir file a path); "+
			"GIT_DIR and GIT_COMMON_DIR are not read", root, filepath.Join(repo.GitTop, ".git"))
	}
	return "", fmt.Errorf("no Git directory found for %s by reading .git there and in the directories above, as vibe-guard does; "+
		"GIT_DIR and GIT_COMMON_DIR are not read, so a work tree without .git cannot be unlocked", root)
}
