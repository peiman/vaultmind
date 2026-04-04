package cmd

import "github.com/spf13/cobra"

var frontmatterCmd = &cobra.Command{
	Use:   "frontmatter",
	Short: "Frontmatter operations (validate)",
}

func init() {
	RootCmd.AddCommand(frontmatterCmd)
}
