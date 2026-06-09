package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignRegistryMetadata defines metadata for the
// `identity sign-registry` command.
var IdentitySignRegistryMetadata = config.CommandMetadata{
	Use:   "sign-registry",
	Short: "Sign a trust-root REGISTRY via the keyless signer (Contract-B)",
	Long: `Read an UNSIGNED registry (stdin or --file), enforce the Contract-B epoch
gate, CANONICALIZE it (RFC 8785 JCS), then sign the canonical bytes via the
signer over its 0600 socket and print the distribution envelope
{registry, root_sig, root_key_epoch} as JSON.

The input is a registry.Registry: { agents[], epoch, valid_from, valid_until },
where each binding is { authorized_origin_daemons[], display_name, key_epoch,
pubkey (base64-std), revoked_at? (absent == live), slug, valid_from,
valid_until }. Every pubkey is re-validated (wrong-length / small-order keys are
rejected) so a hostile key cannot enter a binding.

This CLI is KEYLESS: it NEVER opens the root private-key file. If the signer is
unreachable it FAILS CLOSED with an error (never a silent unsigned result). The
consumer trust gate (root-sig verify, anti-rollback, freshness) runs at load via
VerifyAndLoad, not here.`,
	ConfigPrefix: "app.identitysignregistry",
	FlagOverrides: map[string]string{
		"app.identitysignregistry.file":          "file",
		"app.identitysignregistry.signer_socket": "signer-socket",
	},
}

// IdentitySignRegistryOptions returns config options for `identity sign-registry`.
func IdentitySignRegistryOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysignregistry.file", DefaultValue: "", Description: "Read registry JSON from this file instead of stdin", Type: "string"},
		{Key: "app.identitysignregistry.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignRegistryOptions)
}
