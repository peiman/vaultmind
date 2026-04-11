package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
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

	ret := query.BuildAutoRetrieverFull(vdb.DB)
	defer ret.Cleanup()

	resolver := graph.NewResolver(vdb.DB)
	delta := getConfigValueWithFlags[float64](cmd, "activation-delta", config.KeyExperimentsActivationDelta)
	activationScores := computeActivationScores(cmd.Context(), nil, delta)

	result, err := query.Ask(cmd.Context(), ret.Retriever, resolver, vdb.DB, query.AskConfig{
		Query:            args[0],
		Budget:           getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
		MaxItems:         getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
		SearchLimit:      getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
		ActivationScores: activationScores,
		Embedder:         ret.Embedder,
		ActivationFunc: func(sims map[string]float64) map[string]float64 {
			return computeActivationScores(cmd.Context(), sims, delta)
		},
	})
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	logAskExperiment(cmd, args[0], vaultPath, result)

	if !getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		return query.FormatAsk(result, cmd.OutOrStdout())
	}
	env := envelope.OK("ask", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = vdb.GetIndexHash()
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}
