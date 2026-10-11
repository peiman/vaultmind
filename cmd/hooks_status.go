package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var hooksStatusCmd = func() *cobra.Command {
	c := MustNewCommand(commands.HooksStatusMetadata, runHooksStatus)
	c.Args = cobra.MaximumNArgs(1)
	return c
}()

func init() {
	hooksCmd.AddCommand(hooksStatusCmd)
	setupCommandConfig(hooksStatusCmd)
}

func runHooksStatus(cmd *cobra.Command, args []string) error {
	return runHooksStatusCore(cmd, resolveProjectDir(args),
		getConfigValueWithFlags[string](cmd, "scope", config.KeyAppHooksstatusScope),
		getConfigValueWithFlags[bool](cmd, "json", config.KeyAppHooksstatusJson))
}
