package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var askCmd = MustNewCommand(commands.AskMetadata, runAsk)

func init() {
	MustAddToRoot(askCmd)
}

func runAsk(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind ask <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppAskVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "ask")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	resolver := graph.NewResolver(vdb.DB)
	result, err := query.Ask(&query.FTSRetriever{DB: vdb.DB}, resolver, vdb.DB, query.AskConfig{
		Query:       args[0],
		Budget:      getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
		MaxItems:    getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
		SearchLimit: getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
	})
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	// Log experiment event (non-blocking)
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.VaultPath = vaultPath
		_, _ = session.LogAskEvent(args[0], map[string]any{
			"top_hits": len(result.TopHits),
			"variants": map[string]any{
				"none": map[string]any{"results": []any{}},
			},
		})
	}

	if !getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		return query.FormatAsk(result, cmd.OutOrStdout())
	}
	env := envelope.OK("ask", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = vdb.GetIndexHash()
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}
