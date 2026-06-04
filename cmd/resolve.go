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

var resolveCmd = MustNewCommand(commands.ResolveMetadata, runResolve)

func init() {
	MustAddToRoot(resolveCmd)
}

func runResolve(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind resolve <id-or-title-or-alias>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppResolveVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "resolve")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	return query.RunResolve(vdb.DB, args[0], vaultPath,
		getConfigValueWithFlags[bool](cmd, "json", config.KeyAppResolveJson), cmd.OutOrStdout())
}
