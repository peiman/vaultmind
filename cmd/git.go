package cmd

import "github.com/spf13/cobra"

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Inspect git repository state relevant to vault operations",
	Long: `Query git repository state for the vault directory.

VaultMind mutation policies (writes, merges, episode capture) consult git
state to decide whether an operation is safe. These subcommands expose that
same state so scripts and agents can gate their own decisions on the same
information without re-implementing the detection logic.

Subcommands:
  status   Report branch, dirty state, and merge/rebase status for a vault`,
}

func init() {
	RootCmd.AddCommand(gitCmd)
}
