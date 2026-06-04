package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

var frontmatterSetCmd = MustNewCommand(commands.FrontmatterSetMetadata, runFrontmatterSet)

func init() {
	frontmatterCmd.AddCommand(frontmatterSetCmd)
	setupCommandConfig(frontmatterSetCmd)
}

func runFrontmatterSet(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: frontmatter set <target> <key> <value>")
	}
	return runMutation(cmd, mutation.MutationRequest{
		Op: mutation.OpSet, Target: args[0], Key: args[1], Value: args[2],
	}, "frontmatter set", config.KeyAppFrontmattersetVault, config.KeyAppFrontmattersetJson,
		config.KeyAppFrontmattersetDryRun, config.KeyAppFrontmattersetDiff,
		config.KeyAppFrontmattersetCommit, config.KeyAppFrontmattersetAllowExtra)
}
