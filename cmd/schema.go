package cmd

import "github.com/spf13/cobra"

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Schema operations",
}

func init() {
	RootCmd.AddCommand(schemaCmd)
}
