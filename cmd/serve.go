package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var serveCmd = MustNewCommand(commands.ServeMetadata, runServe)

func init() {
	MustAddToRoot(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	return serveUntilIdle(cmd.Context(), getConfigValueWithFlags[string](cmd, "idle", config.KeyAppServeIdle))
}
