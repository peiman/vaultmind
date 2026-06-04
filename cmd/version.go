// cmd/version.go
// ckeletin:allow-custom-command

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd prints the build version, mirroring the global --version flag.
// Added because `vaultmind version` previously errored with "unknown command"
// while `--help` listed it under Setup — a first-impression papercut surfaced
// by the first knowledge-vault adopter (focalc field report, P2). Checking the
// version is often the very first thing a new user does.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version, commit, and build date",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s version %s, commit %s, built at %s\n",
			binaryName, Version, Commit, Date)
		return err
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
