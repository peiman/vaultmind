package cmd

import (
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var linksInCmd = MustNewCommand(commands.LinksInMetadata, runLinksIn)

func init() {
	linksCmd.AddCommand(linksInCmd)
	setupCommandConfig(linksInCmd)
}

func runLinksIn(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind links in <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppLinksVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "links in")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	return query.RunLinks(vdb.DB, query.LinksConfig{
		Input: args[0], Direction: "in", VaultPath: vaultPath,
		EdgeType:   getConfigValueWithFlags[string](cmd, "edge-type", config.KeyAppLinksEdgeType),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppLinksJson),
		IndexHash:  vdb.GetIndexHash(),
	}, cmd.OutOrStdout())
}
