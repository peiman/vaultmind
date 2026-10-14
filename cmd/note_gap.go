package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var noteGapCmd = MustNewCommand(commands.NoteGapMetadata, runNoteGap)

func init() {
	noteCmd.AddCommand(noteGapCmd)
	setupCommandConfig(noteGapCmd)
	noteGapCmd.Args = cobra.ExactArgs(1)
}

func runNoteGap(cmd *cobra.Command, args []string) error {
	return runGap(cmd, args)
}
