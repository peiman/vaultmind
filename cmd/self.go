package cmd

import (
	"errors"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var selfCmd = MustNewCommand(commands.SelfMetadata, runSelf)

func init() {
	MustAddToRoot(selfCmd)
}

func runSelf(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppSelfVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "self")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	return query.RunSelf(vdb.DB, query.SelfConfig{
		Limit: getConfigValueWithFlags[int](cmd, "limit", config.KeyAppSelfLimit),
	}, cmd.OutOrStdout())
}
