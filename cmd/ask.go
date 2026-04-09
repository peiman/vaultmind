package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

	// Compute activation scores if experiment is enabled
	var activationScores map[string]float64
	if session := experiment.FromContext(cmd.Context()); session != nil {
		expMap := viper.GetStringMap("experiments")
		exps := experiment.ParseExperiments(expMap)
		if actDef, ok := exps["activation"]; ok && actDef.Enabled {
			gamma, _ := experiment.VariantGamma(actDef.Primary)
			params := experiment.DefaultActivationParams(gamma)
			accessedNotes, _ := session.DB.AccessedNoteIDs()
			if len(accessedNotes) > 0 {
				activationScores, _, _ = experiment.ComputeBatchScores(session.DB, accessedNotes, params)
			}
		}
	}

	result, err := query.Ask(retriever, resolver, vdb.DB, query.AskConfig{
		Query:            args[0],
		Budget:           getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
		MaxItems:         getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
		SearchLimit:      getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
		ActivationScores: activationScores,
	})
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	// Log experiment event with shadow variant scores
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
		expMap := viper.GetStringMap("experiments")
		exps := experiment.ParseExperiments(expMap)
		if actDef, ok := exps["activation"]; ok && actDef.Enabled && result.Context != nil {
			accessedNotes, _ := session.DB.AccessedNoteIDs()
			accessMap, _ := session.DB.BatchNoteAccessTimes(accessedNotes)
			windows, _ := session.DB.RecentSessionWindows(100)
			now := time.Now().UTC()
			variantResults := make(map[string]any, len(actDef.AllVariants()))
			for _, variant := range actDef.AllVariants() {
				gamma, _ := experiment.VariantGamma(variant)
				params := experiment.DefaultActivationParams(gamma)
				_, feats := experiment.ScoreFromData(accessedNotes, accessMap, windows, now, params)
				results := make([]any, 0, len(result.Context.Context))
				for rank, item := range result.Context.Context {
					r := map[string]any{"note_id": item.ID, "rank": rank + 1}
					if f, ok := feats[item.ID]; ok {
						r["features"] = f
					}
					results = append(results, r)
				}
				variantResults[variant] = map[string]any{"results": results}
			}
			_, _ = session.LogAskEvent(args[0], map[string]any{
				"primary_variant": actDef.Primary,
				"top_hits":        len(result.TopHits),
				"variants":        variantResults,
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
