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
	memory "github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

var memoryNeighborsCmd = MustNewCommand(commands.MemoryNeighborsMetadata, runMemoryNeighbors)

func init() {
	memoryCmd.AddCommand(memoryNeighborsCmd)
	setupCommandConfig(memoryNeighborsCmd)
}

func runMemoryNeighbors(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind memory neighbors <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMemoryneighborsVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "memory neighbors")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemoryneighborsJson)
	resolver := graph.NewResolver(vdb.DB)
	result, err := memory.Recall(resolver, vdb.DB, memory.RecallConfig{
		Input:         args[0],
		Depth:         getConfigValueWithFlags[int](cmd, "depth", config.KeyAppMemoryneighborsDepth),
		MinConfidence: getConfigValueWithFlags[string](cmd, "min-confidence", config.KeyAppMemoryneighborsMinConfidence),
		MaxNodes:      getConfigValueWithFlags[int](cmd, "max-nodes", config.KeyAppMemoryneighborsMaxNodes),
	})
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "memory neighbors", "neighbors_error", err.Error())
		}
		return fmt.Errorf("neighbors: %w", err)
	}

	// Log note access for experiment outcome linkage (non-blocking).
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
		_, _ = session.LogNoteAccessEvent(args[0], "neighbors")
	}

	if jsonOut {
		env := envelope.OK("memory neighbors", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	return formatRecall(result, cmd.OutOrStdout())
}
