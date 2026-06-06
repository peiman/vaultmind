package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
)

// memory context-pack is DEPRECATED: renamed to `memory pack` (identical
// behavior). This hidden alias prints a one-line notice and delegates to
// runMemoryPack. Kept for ~2 releases.
var memoryContextPackCmd = newDeprecatedAlias(commands.MemoryContextPackMetadata,
	"vaultmind: 'memory context-pack' is deprecated; use 'memory pack' instead",
	runMemoryPack)

func init() {
	memoryCmd.AddCommand(memoryContextPackCmd)
	setupCommandConfig(memoryContextPackCmd)
}
