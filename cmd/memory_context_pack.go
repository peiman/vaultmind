package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/graph"
	memory "github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
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

	activationScores := computeActivationScores(cmd.Context(), nil, 0)

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
		exps := loadExperimentDefs()
		if actDef, ok := exps["activation"]; ok && actDef.Enabled {
			items := make([]rankedItem, len(result.Context))
			for i, item := range result.Context {
				items[i] = rankedItem{ID: item.ID, Rank: i + 1}
			}
			_, _ = session.LogContextPackEvent(map[string]any{
				"primary_variant": actDef.Primary,
				"target_id":       result.TargetID,
				"variants":        buildVariantResults(session, actDef, items),
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
