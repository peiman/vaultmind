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

var memoryRecallCmd = MustNewCommand(commands.MemoryRecallMetadata, runMemoryRecall)

func init() {
	memoryCmd.AddCommand(memoryRecallCmd)
	setupCommandConfig(memoryRecallCmd)
}

func runMemoryRecall(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: memory recall <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMemoryrecallVault)
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return err
	}
	defer vdb.Close()

	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemoryrecallJson)
	resolver := graph.NewResolver(vdb.DB)
	result, err := memory.Recall(resolver, vdb.DB, memory.RecallConfig{
		Input:         args[0],
		Depth:         getConfigValueWithFlags[int](cmd, "depth", config.KeyAppMemoryrecallDepth),
		MinConfidence: getConfigValueWithFlags[string](cmd, "min-confidence", config.KeyAppMemoryrecallMinConfidence),
		MaxNodes:      getConfigValueWithFlags[int](cmd, "max-nodes", config.KeyAppMemoryrecallMaxNodes),
	})
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "memory recall", "recall_error", err.Error())
		}
		return fmt.Errorf("recall: %w", err)
	}

	if jsonOut {
		env := envelope.OK("memory recall", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	return formatRecall(result, cmd.OutOrStdout())
}

func formatRecall(result *memory.RecallResult, w io.Writer) error {
	for _, n := range result.Nodes {
		if n.Distance == 0 {
			if _, err := fmt.Fprintf(w, "%s [%s] %q (depth 0)\n", n.ID, n.Type, n.Title); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "  → %s [%s] %q depth %d\n", n.ID, n.Type, n.Title, n.Distance); err != nil {
				return err
			}
		}
	}
	suffix := ""
	if result.MaxNodesReached {
		suffix = " (max reached)"
	}
	_, err := fmt.Fprintf(w, "%d nodes%s\n", len(result.Nodes), suffix)
	return err
}
