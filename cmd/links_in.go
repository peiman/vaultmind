package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

// links in is DEPRECATED: use `memory links --in`. This hidden alias prints a
// one-line notice and delegates to runLinksDirection with direction "in".
var linksInCmd = newDeprecatedAlias(commands.LinksInMetadata,
	"vaultmind: 'links in' is deprecated; use 'memory links --in' instead",
	func(cmd *cobra.Command, args []string) error {
		return runLinksDirection(cmd, args, "in")
	})

func init() {
	linksCmd.AddCommand(linksInCmd)
	setupCommandConfig(linksInCmd)
}
