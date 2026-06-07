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

// neighborsKeys names the viper keys a neighbors invocation reads its
// depth/min-confidence/max-nodes defaults from. The canonical `memory
// neighbors` path uses the memoryneighbors.* keys (high/50); the deprecated
// `links neighbors` alias uses the linksneighbors.* keys (low/200) so its
// historical defaults are preserved across the merge (M1).
type neighborsKeys struct {
	depthKey         string
	minConfidenceKey string
	maxNodesKey      string
}

// memoryNeighborsKeys is the canonical key set for `memory neighbors`.
var memoryNeighborsKeys = neighborsKeys{
	depthKey:         config.KeyAppMemoryneighborsDepth,
	minConfidenceKey: config.KeyAppMemoryneighborsMinConfidence,
	maxNodesKey:      config.KeyAppMemoryneighborsMaxNodes,
}

func runMemoryNeighbors(cmd *cobra.Command, args []string) error {
	return runNeighborsWithKeys(cmd, args, memoryNeighborsKeys)
}

// runNeighborsWithKeys is the shared neighbors engine. It reads the
// depth/min-confidence/max-nodes defaults from the supplied key set (so the
// deprecated links-neighbors alias keeps its low/200 defaults) and the
// vault/json flags from the memoryneighbors.* keys (shared by both paths since
// the flags carry the same registered values). When a flag was explicitly set
// on the command line, getConfigValueWithFlags returns the flag value
// regardless of which default key was consulted.
func runNeighborsWithKeys(cmd *cobra.Command, args []string, keys neighborsKeys) error {
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
		Depth:         getConfigValueWithFlags[int](cmd, "depth", keys.depthKey),
		MinConfidence: getConfigValueWithFlags[string](cmd, "min-confidence", keys.minConfidenceKey),
		MaxNodes:      getConfigValueWithFlags[int](cmd, "max-nodes", keys.maxNodesKey),
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
