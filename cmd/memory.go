package cmd

import "github.com/spf13/cobra"

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Associative memory queries (recall, related, context-pack)",
}

func init() {
	RootCmd.AddCommand(memoryCmd)
}
