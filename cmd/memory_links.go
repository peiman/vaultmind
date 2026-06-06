package cmd

import (
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var memoryLinksCmd = MustNewCommand(commands.MemoryLinksMetadata, runMemoryLinks)

func init() {
	memoryCmd.AddCommand(memoryLinksCmd)
	setupCommandConfig(memoryLinksCmd)
}

func runMemoryLinks(cmd *cobra.Command, args []string) error {
	return runLinksDirection(cmd, args, linksDirectionFromFlags(cmd))
}

// linksDirectionFromFlags maps the --out/--in/--both flags to a direction.
// Default (no flag or --both) is "both".
func linksDirectionFromFlags(cmd *cobra.Command) string {
	if getConfigValueWithFlags[bool](cmd, "out", config.KeyAppMemorylinksOut) {
		return "out"
	}
	if getConfigValueWithFlags[bool](cmd, "in", config.KeyAppMemorylinksIn) {
		return "in"
	}
	return "both"
}

// runLinksDirection executes the links query for the given direction, reusing
// internal/query.RunLinks for each direction. "both" runs out then in.
func runLinksDirection(cmd *cobra.Command, args []string, direction string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind memory links <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMemorylinksVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "memory links")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	cfg := query.LinksConfig{
		Input:      args[0],
		VaultPath:  vaultPath,
		EdgeType:   getConfigValueWithFlags[string](cmd, "edge-type", config.KeyAppMemorylinksEdgeType),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemorylinksJson),
		IndexHash:  vdb.GetIndexHash(),
	}
	for _, dir := range linkDirections(direction) {
		cfg.Direction = dir
		if err := query.RunLinks(vdb.DB, cfg, cmd.OutOrStdout()); err != nil {
			return err
		}
	}
	return nil
}

// linkDirections expands a requested direction into the ordered set of
// concrete directions to query. "both" -> out then in.
func linkDirections(direction string) []string {
	if direction == "both" {
		return []string{"out", "in"}
	}
	return []string{direction}
}
