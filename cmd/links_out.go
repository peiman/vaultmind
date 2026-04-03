package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var linksOutCmd = MustNewCommand(commands.LinksOutMetadata, runLinksOut)

func init() {
	linksCmd.AddCommand(linksOutCmd)
	setupCommandConfig(linksOutCmd)
}

func runLinksOut(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind links out <id-or-path>")
	}
	return runLinksDirection(cmd, args[0], "out")
}
