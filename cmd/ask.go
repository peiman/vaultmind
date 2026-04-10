package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
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

	retriever, cleanup, err := query.BuildAutoRetriever(vdb.DB)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	resolver := graph.NewResolver(vdb.DB)

	// Step 1: Search — retrieves hits with cosine similarity scores.
	searchLimit := getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit)
	hits, _, err := retriever.Search(context.Background(), args[0], searchLimit, 0, index.SearchFilters{})
	if err != nil {
		return fmt.Errorf("ask: search: %w", err)
	}

	// Step 2: Build similarities from search scores for spreading activation.
	similarities := make(map[string]float64, len(hits))
	for _, hit := range hits {
		similarities[hit.ID] = hit.Score
	}

	// Step 3: Compute activation scores with spreading activation (query similarity).
	activationScores := computeActivationScores(cmd.Context(), similarities)

	// Step 4: Context-pack around the top hit.
	result := &query.AskResult{Query: args[0], TopHits: hits}
	if len(hits) > 0 {
		packResult, packErr := memory.ContextPack(resolver, vdb.DB, memory.ContextPackConfig{
			Input:            hits[0].ID,
			Budget:           getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
			Depth:            1,
			MaxItems:         getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
			Slim:             true,
			ActivationScores: activationScores,
		})
		if packErr == nil {
			result.Context = packResult
		}
	}

	// Log experiment event with shadow variant scores
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
		exps := loadExperimentDefs()
		if actDef, ok := exps["activation"]; ok && actDef.Enabled && result.Context != nil {
			items := make([]rankedItem, len(result.Context.Context))
			for i, item := range result.Context.Context {
				items[i] = rankedItem{ID: item.ID, Rank: i + 1}
			}
			_, _ = session.LogAskEvent(args[0], map[string]any{
				"primary_variant": actDef.Primary,
				"top_hits":        len(result.TopHits),
				"variants":        buildVariantResults(session, actDef, items),
			})
		} else {
			_, _ = session.LogAskEvent(args[0], map[string]any{
				"top_hits": len(result.TopHits),
				"variants": map[string]any{"none": map[string]any{"results": []any{}}},
			})
		}
	}

	if !getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		return query.FormatAsk(result, cmd.OutOrStdout())
	}
	env := envelope.OK("ask", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = vdb.GetIndexHash()
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}
