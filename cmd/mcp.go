package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/mcpserver"
	"github.com/spf13/cobra"
)

var mcpCmd = MustNewCommand(commands.McpMetadata, runMCP)

func init() {
	MustAddToRoot(mcpCmd)
}

func runMCP(cmd *cobra.Command, _ []string) error {
	cfg, err := mcpServerConfig(
		getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMcpVault),
		getConfigValueWithFlags[string](cmd, "vaults", config.KeyAppMcpVaults),
	)
	if err != nil {
		return err
	}
	return mcpserver.Serve(cmd.Context(), cfg)
}
