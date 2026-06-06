package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
)

// memory recall is DEPRECATED: merged into `memory neighbors` (BFS traversal
// with full frontmatter). This hidden alias prints a one-line notice and
// delegates to runMemoryNeighbors. Kept for ~2 releases.
var memoryRecallCmd = newDeprecatedAlias(commands.MemoryRecallMetadata,
	"vaultmind: 'memory recall' is deprecated; use 'memory neighbors' instead",
	runMemoryNeighbors)

func init() {
	memoryCmd.AddCommand(memoryRecallCmd)
	setupCommandConfig(memoryRecallCmd)
}
