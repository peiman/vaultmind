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
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var linksNeighborsCmd = MustNewCommand(commands.LinksNeighborsMetadata, runLinksNeighbors)

func init() {
	linksCmd.AddCommand(linksNeighborsCmd)
	setupCommandConfig(linksNeighborsCmd)
}

func runLinksNeighbors(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: links neighbors <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppLinksneighborsVault)
	vdb, err := cmdutil.OpenVaultDB(vaultPath)
	if err != nil {
		return err
	}
	defer vdb.Close()

	resolver := graph.NewResolver(vdb.DB)
	depth := getConfigValueWithFlags[int](cmd, "depth", config.KeyAppLinksneighborsDepth)
	minConf := getConfigValueWithFlags[string](cmd, "min-confidence", config.KeyAppLinksneighborsMinConfidence)
	maxNodes := getConfigValueWithFlags[int](cmd, "max-nodes", config.KeyAppLinksneighborsMaxNodes)

	result, err := query.Neighbors(resolver, args[0], depth, minConf, maxNodes)
	if err != nil {
		return fmt.Errorf("neighbors: %w", err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppLinksneighborsJson) {
		env := envelope.OK("links neighbors", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	return formatNeighbors(result, cmd.OutOrStdout())
}

func formatNeighbors(result *query.NeighborsResult, w io.Writer) error {
	for _, n := range result.Nodes {
		if n.Distance == 0 {
			if _, err := fmt.Fprintf(w, "%s (depth 0)\n", n.ID); err != nil {
				return err
			}
		} else if n.EdgeFrom != nil {
			if _, err := fmt.Fprintf(w, "  → %s (%s, %s) depth %d\n",
				n.ID, n.EdgeFrom.EdgeType, n.EdgeFrom.Confidence, n.Distance); err != nil {
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
