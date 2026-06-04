package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

var frontmatterUnsetCmd = MustNewCommand(commands.FrontmatterUnsetMetadata, runFrontmatterUnset)

func init() {
	frontmatterCmd.AddCommand(frontmatterUnsetCmd)
	setupCommandConfig(frontmatterUnsetCmd)
}

func runFrontmatterUnset(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: frontmatter unset <target> <key>")
	}
	return runMutation(cmd, mutation.MutationRequest{
		Op: mutation.OpUnset, Target: args[0], Key: args[1],
	}, "frontmatter unset", config.KeyAppFrontmatterunsetVault, config.KeyAppFrontmatterunsetJson,
		config.KeyAppFrontmatterunsetDryRun, config.KeyAppFrontmatterunsetDiff,
		config.KeyAppFrontmatterunsetCommit, "")
}
