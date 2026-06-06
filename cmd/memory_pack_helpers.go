package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	memory "github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

// logPackEvent records the context-pack experiment event (with shadow variant
// scores when an activation experiment is enabled). Non-blocking.
func logPackEvent(cmd *cobra.Command, vaultPath string, result *memory.ContextPackResult) {
	session := experiment.FromContext(cmd.Context())
	if session == nil {
		return
	}
	session.SetVaultPath(vaultPath)
	exps := loadExperimentDefs()
	if actDef, ok := exps["activation"]; ok && actDef.Enabled {
		_, _ = session.LogContextPackEvent(map[string]any{
			"primary_variant": actDef.Primary,
			"target_id":       result.TargetID,
			"variants":        experiment.BuildShadowVariantResults(session, actDef, contextNoteIDs(result.Context)),
		})
		return
	}
	_, _ = session.LogContextPackEvent(map[string]any{
		"target_id":     result.TargetID,
		"context_items": len(result.Context),
		"variants":      map[string]any{"none": map[string]any{"results": []any{}}},
	})
}

// writePackEnvelope encodes the pack result as a JSON envelope.
func writePackEnvelope(cmd *cobra.Command, vaultPath, indexHash string, result *memory.ContextPackResult) error {
	env := envelope.OK("memory pack", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = indexHash
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}

// formatContextPack renders the human-readable pack summary.
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
