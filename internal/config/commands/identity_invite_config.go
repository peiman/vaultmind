package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentityInviteMetadata defines metadata for the `identity invite` command.
var IdentityInviteMetadata = config.CommandMetadata{
	Use:   "invite",
	Short: "Emit an UNSIGNED network invite carrying the trust anchor (Contract-B)",
	Long: `Build a Contract-B agent-network INVITE from the network's ROOT public key and
relay base URL, and print three blocks: the bootstrap TOKEN, the enroll URL (the
token in its fragment), and the FINGERPRINT to share out of band.

The invite is UNSIGNED by design: it CARRIES the network's root anchor (the
ed25519 root public key) itself, so a signature would prove nothing. Authenticity
comes OUT OF BAND — the admin reads the fingerprint (the vmnet1:… network id) to
the member over a TRUSTED channel, and the member confirms it before
` + "`identity enroll`" + ` trusts the anchor.

--root-pubkey is the base64-std value ` + "`identity init`" + ` printed for the root key.
--relay is the relay base URL (e.g. https://chat.acme.com). Both are REQUIRED;
an invalid root pubkey or empty relay FAILS CLOSED with an error and no output.`,
	ConfigPrefix: "app.identityinvite",
	FlagOverrides: map[string]string{
		"app.identityinvite.root_pubkey": "root-pubkey",
		"app.identityinvite.relay":       "relay",
	},
}

// IdentityInviteOptions returns config options for `identity invite`.
func IdentityInviteOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identityinvite.root_pubkey", DefaultValue: "", Description: "Network ROOT public key (base64-std of the 32-byte ed25519 key; required)", Type: "string"},
		{Key: "app.identityinvite.relay", DefaultValue: "", Description: "Relay base URL, e.g. https://chat.acme.com (required)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentityInviteOptions)
}
