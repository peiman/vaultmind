package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var memoryLinksCmd = MustNewCommand(commands.MemoryLinksMetadata, runMemoryLinks)

func init() {
	memoryCmd.AddCommand(memoryLinksCmd)
	setupCommandConfig(memoryLinksCmd)
	// Tri-state direction is mutually exclusive: --out, --in, and --both
	// cannot be combined. Cobra rejects the combination before RunE.
	memoryLinksCmd.MarkFlagsMutuallyExclusive("out", "in", "both")
}

func runMemoryLinks(cmd *cobra.Command, args []string) error {
	return runLinksDirection(cmd, args, linksDirectionFromFlags(cmd))
}

// linksDirectionFromFlags maps the --out/--in/--both flags to a direction.
// Default (no flag) is "both"; an explicit --both also yields "both" so the
// flag is not dead. --out/--in/--both are mutually exclusive (enforced by
// MarkFlagsMutuallyExclusive), so at most one is set.
func linksDirectionFromFlags(cmd *cobra.Command) string {
	if getConfigValueWithFlags[bool](cmd, "out", config.KeyAppMemorylinksOut) {
		return "out"
	}
	if getConfigValueWithFlags[bool](cmd, "in", config.KeyAppMemorylinksIn) {
		return "in"
	}
	if getConfigValueWithFlags[bool](cmd, "both", config.KeyAppMemorylinksBoth) {
		return "both"
	}
	return "both"
}

// runLinksDirection executes the links query for the given direction. Single
// directions ("out"/"in") render one envelope via query.RunLinks. "both"
// aggregates BOTH directions: in JSON it is ONE combined envelope (out before
// in); in human output it is two section-headed blocks.
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
		Direction:  direction,
		VaultPath:  vaultPath,
		EdgeType:   getConfigValueWithFlags[string](cmd, "edge-type", config.KeyAppMemorylinksEdgeType),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemorylinksJson),
		IndexHash:  vdb.GetIndexHash(),
	}

	if direction == "both" {
		return runLinksBoth(cmd, vdb, cfg)
	}
	return query.RunLinks(vdb.DB, cfg, cmd.OutOrStdout())
}

// runLinksBoth renders both directions. JSON: one combined envelope so stdout
// is a single valid JSON value. Human: an "outbound:" block then an "inbound:"
// block so directions are distinguishable.
func runLinksBoth(cmd *cobra.Command, vdb *cmdutil.VaultDB, cfg query.LinksConfig) error {
	both, _, err := query.CollectBoth(vdb.DB, cfg)
	if err != nil {
		if cfg.JSONOutput {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "memory links",
				"resolution_failed", err.Error())
		}
		return err
	}

	w := cmd.OutOrStdout()
	if cfg.JSONOutput {
		env := envelope.OK("memory links", both)
		env.Meta.VaultPath = cfg.VaultPath
		env.Meta.IndexHash = cfg.IndexHash
		return json.NewEncoder(w).Encode(env)
	}

	if _, err := fmt.Fprintln(w, "outbound:"); err != nil {
		return err
	}
	for _, l := range both.Out.Links {
		id := l.TargetRaw
		if l.TargetID != nil {
			id = *l.TargetID
		}
		if _, err := fmt.Fprintf(w, "  %-20s %-20s %s\n", id, l.EdgeType, l.Confidence); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "inbound:"); err != nil {
		return err
	}
	for _, l := range both.In.Links {
		if _, err := fmt.Fprintf(w, "  %-20s %-20s %s\n", l.SourceID, l.EdgeType, l.Confidence); err != nil {
			return err
		}
	}
	return nil
}
