package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var linksInCmd = MustNewCommand(commands.LinksInMetadata, runLinksIn)

func init() {
	linksCmd.AddCommand(linksInCmd)
	setupCommandConfig(linksInCmd)
}

func runLinksIn(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind links in <id-or-path>")
	}
	return runLinksDirection(cmd, args[0], "in")
}
