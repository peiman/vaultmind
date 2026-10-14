package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var noteProposeCmd = MustNewCommand(commands.NoteProposeMetadata, runNotePropose)

func init() {
	noteCmd.AddCommand(noteProposeCmd)
	setupCommandConfig(noteProposeCmd)
	noteProposeCmd.Flags().StringArray("source", []string{}, "Source URL or path inside the vault or its repository (repeatable)")
	trackRepeats(noteProposeCmd, "source")
}

func runNotePropose(cmd *cobra.Command, _ []string) error {
	return runPropose(cmd)
}
