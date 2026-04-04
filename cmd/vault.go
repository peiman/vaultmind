package cmd

import "github.com/spf13/cobra"

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Vault operations",
}

func init() {
	RootCmd.AddCommand(vaultCmd)
}
