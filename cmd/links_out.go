package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var linksOutCmd = MustNewCommand(commands.LinksOutMetadata, runLinksOut)

func init() {
	linksCmd.AddCommand(linksOutCmd)
	setupCommandConfig(linksOutCmd)
}

func runLinksOut(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind links out <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppLinksVault)
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return err
	}
	defer vdb.Close()

	return query.RunLinks(vdb.DB, query.LinksConfig{
		Input: args[0], Direction: "out", VaultPath: vaultPath,
		EdgeType:   getConfigValueWithFlags[string](cmd, "edge-type", config.KeyAppLinksEdgeType),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppLinksJson),
		IndexHash:  vdb.GetIndexHash(),
	}, cmd.OutOrStdout())
}
