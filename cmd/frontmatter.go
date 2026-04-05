package cmd

import "github.com/spf13/cobra"

var frontmatterCmd = &cobra.Command{
	Use:   "frontmatter",
	Short: "Frontmatter operations (validate, set, unset, merge, normalize)",
}

func init() {
	RootCmd.AddCommand(frontmatterCmd)
}
