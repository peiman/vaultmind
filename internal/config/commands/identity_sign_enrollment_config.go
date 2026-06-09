package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignEnrollmentMetadata defines metadata for the
// `identity sign-enrollment` command.
var IdentitySignEnrollmentMetadata = config.CommandMetadata{
	Use:   "sign-enrollment",
	Short: "Self-sign an agent ENROLLMENT request via the keyless signer (Contract-B)",
	Long: `Read an agent-enrollment request (stdin or --file), enforce the Contract-B
enrollment gates, CANONICALIZE the signed subset (RFC 8785 JCS), then sign the
canonical bytes via the signer over its 0600 socket and print {sig, pubkey} as
JSON.

The signed subset is { alg_version, created, display_name, key_epoch,
network_id, nonce, pubkey, slug, transport_endpoint? (optional), transport_pubkey }.
The pubkey field IS the enrolling agent's ed25519 identity key AND the
verification key: the self-signature is PROOF-OF-POSSESSION of that key. It does
NOT prove the slug/identity is authorized — an admin separately decides to add
the binding to the trust-root registry (the out-of-band step).

This CLI is KEYLESS: it NEVER opens the private-key file. If the signer is
unreachable it FAILS CLOSED with an error (never a silent unsigned result).`,
	ConfigPrefix: "app.identitysignenrollment",
	FlagOverrides: map[string]string{
		"app.identitysignenrollment.file":          "file",
		"app.identitysignenrollment.signer_socket": "signer-socket",
	},
}

// IdentitySignEnrollmentOptions returns config options for
// `identity sign-enrollment`.
func IdentitySignEnrollmentOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysignenrollment.file", DefaultValue: "", Description: "Read enrollment request JSON from this file instead of stdin", Type: "string"},
		{Key: "app.identitysignenrollment.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignEnrollmentOptions)
}
