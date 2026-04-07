package cmd

import "github.com/spf13/cobra"

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Lint and fix vault content",
}

func init() {
	RootCmd.AddCommand(lintCmd)
}
