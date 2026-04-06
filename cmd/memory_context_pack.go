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
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return err
	}
	defer vdb.Close()

	resolver := graph.NewResolver(vdb.DB)
	result, err := memory.ContextPack(resolver, vdb.DB, memory.ContextPackConfig{
		Input:  args[0],
		Budget: getConfigValueWithFlags[int](cmd, "budget", config.KeyAppMemorycontextpackBudget),
	})
	if err != nil {
		return fmt.Errorf("context-pack: %w", err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemorycontextpackJson) {
		env := envelope.OK("memory context-pack", result)
		env.Meta.VaultPath = vaultPath
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
