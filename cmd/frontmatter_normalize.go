package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

var frontmatterNormalizeCmd = MustNewCommand(commands.FrontmatterNormalizeMetadata, runFrontmatterNormalize)

func init() {
	frontmatterCmd.AddCommand(frontmatterNormalizeCmd)
	setupCommandConfig(frontmatterNormalizeCmd)
}

func runFrontmatterNormalize(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	req := mutation.MutationRequest{Op: mutation.OpNormalize, Target: target}
	req.StripTime = getConfigValueWithFlags[bool](cmd, "strip-time", config.KeyAppFrontmatternormalizeStripTime)
	return runMutation(cmd, req,
		"frontmatter normalize", config.KeyAppFrontmatternormalizeVault, config.KeyAppFrontmatternormalizeJson,
		config.KeyAppFrontmatternormalizeDryRun, config.KeyAppFrontmatternormalizeDiff,
		config.KeyAppFrontmatternormalizeCommit, "")
}
