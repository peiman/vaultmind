package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// VaultStatusMetadata defines the metadata for the vault status command.
var VaultStatusMetadata = config.CommandMetadata{
	Use:          "status",
	Short:        "Vault status summary (cold-start)",
	Long:         "Single-call summary combining note counts, type registry, index freshness, and validation issues.",
	ConfigPrefix: "app.vaultstatus",
	FlagOverrides: map[string]string{
		"app.vaultstatus.vault": "vault",
		"app.vaultstatus.json":  "json",
	},
}

// VaultStatusOptions returns configuration options for vault status.
func VaultStatusOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.vaultstatus.vault", DefaultValue: ".", Description: "Path to vault root", Type: "string"},
		{Key: "app.vaultstatus.json", DefaultValue: false, Description: "Output in JSON format", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(VaultStatusOptions)
}
