package cmd

import "github.com/spf13/cobra"

var experimentCmd = &cobra.Command{
	Use:   "experiment",
	Short: "Experiment tracking and reporting",
}

func init() {
	RootCmd.AddCommand(experimentCmd)
}
