package cmd

import (
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/graph"
	memory "github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

var memoryPackCmd = MustNewCommand(commands.MemoryPackMetadata, runMemoryPack)

func init() {
	memoryCmd.AddCommand(memoryPackCmd)
	setupCommandConfig(memoryPackCmd)
}

func runMemoryPack(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind memory pack <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMemorypackVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "memory pack")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemorypackJson)
	resolver := graph.NewResolver(vdb.DB)
	result, err := memory.ContextPack(resolver, vdb.DB, memory.ContextPackConfig{
		Input:            args[0],
		Budget:           getConfigValueWithFlags[int](cmd, "budget", config.KeyAppMemorypackBudget),
		Depth:            getConfigValueWithFlags[int](cmd, "depth", config.KeyAppMemorypackDepth),
		MaxItems:         getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppMemorypackMaxItems),
		Slim:             getConfigValueWithFlags[bool](cmd, "slim", config.KeyAppMemorypackSlim),
		ActivationScores: computeActivationScores(cmd.Context(), nil, 0),
	})
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "memory pack", "pack_error", err.Error())
		}
		return fmt.Errorf("pack: %w", err)
	}

	logPackEvent(cmd, vaultPath, result)

	if jsonOut {
		return writePackEnvelope(cmd, vaultPath, vdb.GetIndexHash(), result)
	}
	return formatContextPack(result, cmd.OutOrStdout())
}
