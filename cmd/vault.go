package cmd

import "github.com/spf13/cobra"

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Vault operations",
	Long: `Inspect and diagnose the vault itself — note counts, index freshness, and validation issues.

Reach for vault subcommands when you need a top-level view of vault health rather
than retrieving specific notes. Use ask, search, or note for content retrieval.

SUBCOMMANDS

  status   Print a cold-start summary: total notes, domain vs unstructured split,
           type-registry size, and issue counts (errors, warnings). Accepts --json
           for machine-readable output.

EXAMPLES

  vaultmind vault status --vault ./my-vault          # human-readable health summary
  vaultmind vault status --vault ./my-vault --json   # machine-readable envelope`,
}

func init() {
	RootCmd.AddCommand(vaultCmd)
}
