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

var searchCmd = MustNewCommand(commands.SearchMetadata, runSearch)

func init() {
	MustAddToRoot(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind search <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppSearchVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "search")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	retriever := &query.FTSRetriever{DB: vdb.DB}
	return query.RunSearch(retriever, query.SearchConfig{
		Query:      args[0],
		Limit:      getConfigValueWithFlags[int](cmd, "limit", config.KeyAppSearchLimit),
		Offset:     getConfigValueWithFlags[int](cmd, "offset", config.KeyAppSearchOffset),
		TypeFilter: getConfigValueWithFlags[string](cmd, "type", config.KeyAppSearchType),
		TagFilter:  getConfigValueWithFlags[string](cmd, "tag", config.KeyAppSearchTag),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSearchJson),
		VaultPath:  vaultPath,
	}, cmd.OutOrStdout())
}
