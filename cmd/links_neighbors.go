package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
)

// links neighbors is DEPRECATED: merged into `memory neighbors` (BFS traversal
// with full frontmatter). This hidden alias prints a one-line notice and
// delegates to runMemoryNeighbors. Kept for ~2 releases.
var linksNeighborsCmd = newDeprecatedAlias(commands.LinksNeighborsMetadata,
	"vaultmind: 'links neighbors' is deprecated; use 'memory neighbors' instead",
	runMemoryNeighbors)

func init() {
	linksCmd.AddCommand(linksNeighborsCmd)
	setupCommandConfig(linksNeighborsCmd)
}
