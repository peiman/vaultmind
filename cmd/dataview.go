package cmd

import "github.com/spf13/cobra"

var dataviewCmd = &cobra.Command{
	Use:   "dataview",
	Short: "Manage generated regions (lint, render)",
}

func init() {
	RootCmd.AddCommand(dataviewCmd)
}
