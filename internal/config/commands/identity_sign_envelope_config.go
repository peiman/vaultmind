package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignEnvelopeMetadata defines metadata for the
// `identity sign-envelope` command.
var IdentitySignEnvelopeMetadata = config.CommandMetadata{
	Use:   "sign-envelope",
	Short: "Sign a chat MESSAGE envelope via the keyless signer (Contract-B slice 5)",
	Long: `Read a chat-message envelope (stdin or --file), enforce the Contract-B
signing gates, CANONICALIZE the signed subset (RFC 8785 JCS), then sign the
canonical bytes via the signer over its 0600 socket and print
{sig, from_pubkey, key_epoch} as JSON.

The signed subset is { alg_version, body, from_agent, key_epoch, nonce, room?,
seq, to_agent? (exactly one), ts }. from_pubkey is DERIVED, not signed. The
transport fields (id, sig, from_pubkey, receive_ts, ioguard_verdict,
origin_daemon) are excluded from the signed bytes.

This CLI is KEYLESS: it NEVER opens the private-key file. If the signer is
unreachable it FAILS CLOSED with an error (never a silent unsigned result).
Anti-replay (seq high-water + nonce-unseen) is the receiving daemon's job.`,
	ConfigPrefix: "app.identitysignenvelope",
	FlagOverrides: map[string]string{
		"app.identitysignenvelope.file":          "file",
		"app.identitysignenvelope.signer_socket": "signer-socket",
		"app.identitysignenvelope.from_pubkey":   "from-pubkey",
	},
}

// IdentitySignEnvelopeOptions returns config options for `identity sign-envelope`.
func IdentitySignEnvelopeOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysignenvelope.file", DefaultValue: "", Description: "Read envelope JSON from this file instead of stdin", Type: "string"},
		{Key: "app.identitysignenvelope.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
		{Key: "app.identitysignenvelope.from_pubkey", DefaultValue: "", Description: "Signer public key (base64) stamped as the from_pubkey hint; not signed", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignEnvelopeOptions)
}
