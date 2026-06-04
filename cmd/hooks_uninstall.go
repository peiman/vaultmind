package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var hooksUninstallCmd = func() *cobra.Command {
	c := MustNewCommand(commands.HooksUninstallMetadata, runHooksUninstall)
	c.Args = cobra.MaximumNArgs(1)
	return c
}()

func init() {
	hooksCmd.AddCommand(hooksUninstallCmd)
	setupCommandConfig(hooksUninstallCmd)
}

func runHooksUninstall(cmd *cobra.Command, args []string) error {
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppHooksuninstallJson)
	local := getConfigValueWithFlags[bool](cmd, "local", config.KeyAppHooksuninstallLocal)
	removeScripts := getConfigValueWithFlags[bool](cmd, "remove-scripts", config.KeyAppHooksuninstallRemovescripts)
	return runHooksUninstallCore(cmd, resolveProjectDir(args), jsonOut, local, removeScripts)
}
