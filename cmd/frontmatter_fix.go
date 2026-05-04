package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var frontmatterFixCmd = MustNewCommand(commands.FrontmatterFixMetadata, runFrontmatterFix)

func init() {
	frontmatterCmd.AddCommand(frontmatterFixCmd)
	setupCommandConfig(frontmatterFixCmd)
}

func runFrontmatterFix(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppFrontmatterfixVault)
	apply := getConfigValueWithFlags[bool](cmd, "apply", config.KeyAppFrontmatterfixApply)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppFrontmatterfixJson)
	return runFrontmatterFixCore(cmd, vaultPath, apply, jsonOut)
}
