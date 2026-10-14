package cmd

import (
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var noteAcceptCmd = MustNewCommand(commands.NoteAcceptMetadata, runNoteAccept)

func init() {
	noteCmd.AddCommand(noteAcceptCmd)
	setupCommandConfig(noteAcceptCmd)
}

func runNoteAccept(cmd *cobra.Command, args []string) error {
	return runAccept(cmd, args)
}
