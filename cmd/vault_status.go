package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

// vault status is DEPRECATED: doctor is now the single vault-health hub, and
// the cold-start view it produced is `doctor --summary`. This hidden alias
// prints a one-line stderr notice and delegates to the doctor summary path
// (forcing summaryOnly=true). It keeps its own app.vaultstatus.* flags so the
// existing --vault/--json invocations resolve unchanged. Kept for ~2 releases.
var vaultStatusCmd = newDeprecatedAlias(commands.VaultStatusMetadata,
	"vaultmind: 'vault status' is deprecated; use 'doctor --summary' instead",
	runVaultStatus)

func init() {
	vaultCmd.AddCommand(vaultStatusCmd)
	setupCommandConfig(vaultStatusCmd)
}

// runVaultStatus resolves the alias's own vault/json flags and delegates to
// the shared doctor health-hub engine with the summary view forced on.
func runVaultStatus(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppVaultstatusVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppVaultstatusJson)
	return runDoctorCore(cmd, vaultPath, jsonOut, true)
}
