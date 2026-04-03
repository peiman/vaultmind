package cmd

import "github.com/spf13/cobra"

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Note operations (get, create)",
}

func init() {
	RootCmd.AddCommand(noteCmd)
}
