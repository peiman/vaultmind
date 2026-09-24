package cmd

import (
	"fmt"
	"os"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var arcReviewCmd = MustNewCommand(commands.ArcReviewMetadata, runArcReviewCmd)

func init() {
	arcCmd.AddCommand(arcReviewCmd)
}

// runArcReviewCmd shows the review queue or records judgements. See
// commands.ArcReviewMetadata.
func runArcReviewCmd(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppArcReviewVault)
	if info, err := os.Stat(vaultPath); err != nil || !info.IsDir() {
		return fmt.Errorf("vault %q does not exist or is not a directory", vaultPath)
	}
	return runArcReview(cmd.OutOrStdout(), newArcReviewRequest(vaultPath,
		getConfigValueWithFlags[string](cmd, "episode", config.KeyAppArcReviewEpisode),
		getConfigValueWithFlags[string](cmd, "mark-reviewed", config.KeyAppArcReviewMarkReviewed),
		getConfigValueWithFlags[bool](cmd, "json", config.KeyAppArcReviewJson)))
}
