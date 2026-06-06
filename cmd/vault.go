package cmd

import "github.com/spf13/cobra"

// vaultCmd is the DEPRECATED `vault` parent. It only ever hosted `status`,
// whose role is now the doctor health hub (`doctor --summary`). The parent is
// hidden from the root listing; its single subcommand survives as a hidden
// deprecated alias that delegates to the doctor summary path. Kept for ~2
// releases.
var vaultCmd = &cobra.Command{
	Use:    "vault",
	Short:  "Deprecated: use 'doctor' / 'doctor --summary'",
	Hidden: true,
}

func init() {
	RootCmd.AddCommand(vaultCmd)
}
