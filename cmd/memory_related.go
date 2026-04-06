package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	memory "github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

var memoryRelatedCmd = MustNewCommand(commands.MemoryRelatedMetadata, runMemoryRelated)

func init() {
	memoryCmd.AddCommand(memoryRelatedCmd)
	setupCommandConfig(memoryRelatedCmd)
}

func runMemoryRelated(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: memory related <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMemoryrelatedVault)
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return err
	}
	defer vdb.Close()

	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemoryrelatedJson)
	resolver := graph.NewResolver(vdb.DB)
	result, err := memory.Related(resolver, vdb.DB, memory.RelatedConfig{
		Input: args[0],
		Mode:  getConfigValueWithFlags[string](cmd, "mode", config.KeyAppMemoryrelatedMode),
	})
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "memory related", "related_error", err.Error())
		}
		return fmt.Errorf("related: %w", err)
	}

	if jsonOut {
		env := envelope.OK("memory related", result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	return formatRelated(result, cmd.OutOrStdout())
}

func formatRelated(result *memory.RelatedResult, w io.Writer) error {
	for _, item := range result.Related {
		if _, err := fmt.Fprintf(w, "  %s [%s] %q  edge=%s confidence=%s\n",
			item.ID, item.Type, item.Title, item.EdgeType, item.Confidence); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "%d related (mode: %s)\n", len(result.Related), result.Mode)
	return err
}
