package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var notePinnedCmd = MustNewCommand(commands.NotePinnedMetadata, runNotePinned)

func init() {
	noteCmd.AddCommand(notePinnedCmd)
	setupCommandConfig(notePinnedCmd)
}

func runNotePinned(cmd *cobra.Command, _ []string) error {
	return executeNotePinned(cmd)
}
