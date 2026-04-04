package cmd

import "github.com/spf13/cobra"

var linksCmd = &cobra.Command{
	Use:   "links",
	Short: "Link operations (out, in)",
}

func init() {
	RootCmd.AddCommand(linksCmd)
}
