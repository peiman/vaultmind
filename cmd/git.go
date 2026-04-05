package cmd

import "github.com/spf13/cobra"

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git integration",
}

func init() {
	RootCmd.AddCommand(gitCmd)
}
