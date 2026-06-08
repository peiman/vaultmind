package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentityInitMetadata defines metadata for the `identity init` command.
var IdentityInitMetadata = config.CommandMetadata{
	Use:   "init",
	Short: "Mint an agent keypair and seal the private key to the signer",
	Long: `Mint a per-agent ed25519 keypair, SEAL the private key to the signer's 0600
key file, and print the PUBLIC key.

The private key is NEVER printed or logged. Re-running refuses to overwrite an
existing sealed key.

DEV-INTERIM: the sealed key is a raw 0600 file. Secure-Enclave wrapping and a
dedicated service uid are deferred (see internal/identity/signer).`,
	ConfigPrefix: "app.identityinit",
	FlagOverrides: map[string]string{
		"app.identityinit.signer_key": "signer-key",
	},
}

// IdentityInitOptions returns config options for `identity init`.
func IdentityInitOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identityinit.signer_key", DefaultValue: "", Description: "Sealed signer key path (default: XDG data dir)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentityInitOptions)
}
