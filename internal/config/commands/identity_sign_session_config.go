package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignSessionMetadata defines metadata for `identity sign-session`.
var IdentitySignSessionMetadata = config.CommandMetadata{
	Use:   "sign-session",
	Short: "Sign a hub session statement via the keyless signer",
	Long: `Sign a hub-session statement for the agent enrolled in this project and
print one JSON line: {"statement","sig","pubkey"}.

The signed bytes are the UTF-8 tag "vaultmind-hub-session-v1:" immediately
followed by the JCS canonical statement. purpose is hub-session. slug and
key_epoch are this agent's own registry binding; there is no flag to name
another agent. ts is the time of signing.

The challenge and network id are the values the hub issued. This CLI is
KEYLESS: it never opens the private-key file. A bad challenge, a missing
enrollment, or an unreachable signer prints nothing and exits non-zero.

The hub, not this command, checks that the challenge is single-use and
unexpired, that network_id is its own, and that ts is inside its window.`,
	ConfigPrefix: "app.identitysignsession",
	FlagOverrides: map[string]string{
		"app.identitysignsession.challenge":     "challenge",
		"app.identitysignsession.network_id":    "network-id",
		"app.identitysignsession.signer_socket": "signer-socket",
	},
}

// IdentitySignSessionOptions returns config options for `identity sign-session`.
func IdentitySignSessionOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysignsession.challenge", DefaultValue: "", Description: "Hub challenge, standard base64 of 16 to 64 bytes", Type: "string"},
		{Key: "app.identitysignsession.network_id", DefaultValue: "", Description: "Network id the hub issued with the challenge", Type: "string"},
		{Key: "app.identitysignsession.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignSessionOptions)
}
