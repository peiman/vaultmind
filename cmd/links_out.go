package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

// links out is DEPRECATED: use `memory links --out`. This hidden alias prints
// a one-line notice and delegates to runLinksDirection with direction "out".
var linksOutCmd = newDeprecatedAlias(commands.LinksOutMetadata,
	"vaultmind: 'links out' is deprecated; use 'memory links --out' instead",
	func(cmd *cobra.Command, args []string) error {
		return runLinksDirection(cmd, args, "out")
	})

func init() {
	linksCmd.AddCommand(linksOutCmd)
	setupCommandConfig(linksOutCmd)
}
