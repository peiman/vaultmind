package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var hooksInstallCmd = func() *cobra.Command {
	c := MustNewCommand(commands.HooksInstallMetadata, runHooksInstall)
	c.Args = cobra.MaximumNArgs(1)
	return c
}()

func init() {
	hooksCmd.AddCommand(hooksInstallCmd)
	setupCommandConfig(hooksInstallCmd)
}

func runHooksInstall(cmd *cobra.Command, args []string) error {
	return runHooksInstallCore(cmd, hooksInstallParams{
		projectDir: resolveProjectDir(args),
		force:      getConfigValueWithFlags[bool](cmd, "force", config.KeyAppHooksinstallForce),
		jsonOut:    getConfigValueWithFlags[bool](cmd, "json", config.KeyAppHooksinstallJson),
		only:       getConfigValueWithFlags[string](cmd, "only", config.KeyAppHooksinstallOnly),
		vault:      getConfigValueWithFlags[string](cmd, "vault", config.KeyAppHooksinstallVault),
		merge:      getConfigValueWithFlags[bool](cmd, "merge", config.KeyAppHooksinstallMerge),
		local:      getConfigValueWithFlags[bool](cmd, "local", config.KeyAppHooksinstallLocal),
		dryRun:     getConfigValueWithFlags[bool](cmd, "dry-run", config.KeyAppHooksinstallDryrun),
	})
}
