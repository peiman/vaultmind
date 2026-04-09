package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/graph"
	memory "github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var memoryContextPackCmd = MustNewCommand(commands.MemoryContextPackMetadata, runMemoryContextPack)

func init() {
	memoryCmd.AddCommand(memoryContextPackCmd)
	setupCommandConfig(memoryContextPackCmd)
}

func runMemoryContextPack(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: memory context-pack <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMemorycontextpackVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "memory context-pack")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemorycontextpackJson)
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

	result, err := memory.ContextPack(resolver, vdb.DB, memory.ContextPackConfig{
		Input:            args[0],
		Budget:           getConfigValueWithFlags[int](cmd, "budget", config.KeyAppMemorycontextpackBudget),
		Depth:            getConfigValueWithFlags[int](cmd, "depth", config.KeyAppMemorycontextpackDepth),
		MaxItems:         getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppMemorycontextpackMaxItems),
		Slim:             getConfigValueWithFlags[bool](cmd, "slim", config.KeyAppMemorycontextpackSlim),
		ActivationScores: activationScores,
	})
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "memory context-pack", "context_pack_error", err.Error())
		}
		return fmt.Errorf("context-pack: %w", err)
	}

	// Log experiment event with shadow variant scores
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
		expMap := viper.GetStringMap("experiments")
		exps := experiment.ParseExperiments(expMap)
		if actDef, ok := exps["activation"]; ok && actDef.Enabled {
			accessedNotes, _ := session.DB.AccessedNoteIDs()
			accessMap, _ := session.DB.BatchNoteAccessTimes(accessedNotes)
			windows, _ := session.DB.RecentSessionWindows(100)
			now := time.Now().UTC()
			variantResults := make(map[string]any, len(actDef.AllVariants()))
			for _, variant := range actDef.AllVariants() {
				gamma, _ := experiment.VariantGamma(variant)
				params := experiment.DefaultActivationParams(gamma)
				_, feats := experiment.ScoreFromData(accessedNotes, accessMap, windows, now, params, nil)
				results := make([]any, 0, len(result.Context))
				for rank, item := range result.Context {
					r := map[string]any{"note_id": item.ID, "rank": rank + 1}
					if f, ok := feats[item.ID]; ok {
						r["features"] = f
					}
					results = append(results, r)
				}
				variantResults[variant] = map[string]any{"results": results}
			}
			_, _ = session.LogContextPackEvent(map[string]any{
				"primary_variant": actDef.Primary,
				"target_id":       result.TargetID,
				"variants":        variantResults,
			})
		} else {
			_, _ = session.LogContextPackEvent(map[string]any{
				"target_id":     result.TargetID,
				"context_items": len(result.Context),
				"variants":      map[string]any{"none": map[string]any{"results": []any{}}},
			})
		}
	}

	if jsonOut {
		env := envelope.OK("memory context-pack", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	return formatContextPack(result, cmd.OutOrStdout())
}

func formatContextPack(result *memory.ContextPackResult, w io.Writer) error {
	if result.Target != nil {
		if _, err := fmt.Fprintf(w, "target: %s\n", result.Target.ID); err != nil {
			return err
		}
	}
	truncStr := ""
	if result.Truncated {
		truncStr = " (truncated)"
	}
	if _, err := fmt.Fprintf(w, "tokens: %d / %d%s\n", result.UsedTokens, result.BudgetTokens, truncStr); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "%d context items\n", len(result.Context))
	return err
}
