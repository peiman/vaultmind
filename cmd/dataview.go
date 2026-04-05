package cmd

import "github.com/spf13/cobra"

var dataviewCmd = &cobra.Command{
	Use:   "dataview",
	Short: "Managed generated regions",
}

func init() {
	RootCmd.AddCommand(dataviewCmd)
}
