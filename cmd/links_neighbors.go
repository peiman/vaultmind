package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

// linksNeighborsKeys preserves the OLD `links neighbors` defaults (depth 1,
// min-confidence low, max-nodes 200) when the merged `memory neighbors` engine
// runs. The canonical `memory neighbors` defaults are high/50 — narrowing the
// alias to those would be a silent back-compat break, so the alias keeps
// reading its own linksneighbors.* keys (M1).
var linksNeighborsKeys = neighborsKeys{
	depthKey:         config.KeyAppLinksneighborsDepth,
	minConfidenceKey: config.KeyAppLinksneighborsMinConfidence,
	maxNodesKey:      config.KeyAppLinksneighborsMaxNodes,
}

// links neighbors is DEPRECATED: merged into `memory neighbors` (BFS traversal
// with full frontmatter). This hidden alias prints a one-line notice and
// delegates to the shared neighbors engine — but with the alias's OWN default
// keys so an unchanged `links neighbors <id>` still uses low/200, not the
// canonical high/50.
var linksNeighborsCmd = newDeprecatedAlias(commands.LinksNeighborsMetadata,
	"vaultmind: 'links neighbors' is deprecated; use 'memory neighbors' instead",
	func(cmd *cobra.Command, args []string) error {
		return runNeighborsWithKeys(cmd, args, linksNeighborsKeys)
	})

func init() {
	linksCmd.AddCommand(linksNeighborsCmd)
	setupCommandConfig(linksNeighborsCmd)
}
