package cmd

import "github.com/spf13/cobra"

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Note operations (get, mget, create, propose, accept, gap)",
}

func init() {
	RootCmd.AddCommand(noteCmd)
}
