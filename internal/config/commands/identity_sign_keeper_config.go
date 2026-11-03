package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentitySignKeeperMetadata defines metadata for `identity sign-keeper`.
var IdentitySignKeeperMetadata = config.CommandMetadata{
	Use:   "sign-keeper",
	Short: "Sign a keeper control request, read grant, or revocation via the keyless signer",
	Long: `Sign an owner's keeper document from stdin or --file.
keeper.control requests an action from a keeper.
keeper.grant lets another identity read named keepers until expiry.
keeper.grant_revoke permanently revokes a read grant.

Start the keyless signer first with vaultmind identity signer.
The CLI never opens a private key. An unreachable signer fails closed.
--stamp fills an absent issued_at with the current Unix time.
For control requests and revocations, it also fills an absent nonce with
18 random bytes in standard padded base64. Present values are kept.
Grants have no nonce. Without --stamp, all fields must be supplied.

Output is one JSON line: {"document": <canonical JSON object>, "sig": "<base64 signature>"}.
The document contains exactly the canonical bytes that were signed.
The keeper's watchdog checks freshness (300 s), nonce single-use,
the watchdog start time and ownership.`,
	ConfigPrefix: "app.identitysignkeeper",
	FlagOverrides: map[string]string{
		"app.identitysignkeeper.file":          "file",
		"app.identitysignkeeper.signer_socket": "signer-socket",
		"app.identitysignkeeper.stamp":         "stamp",
	},
}

// IdentitySignKeeperOptions returns config options for `identity sign-keeper`.
func IdentitySignKeeperOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identitysignkeeper.file", DefaultValue: "", Description: "Read keeper document JSON from this file instead of stdin", Type: "string"},
		{Key: "app.identitysignkeeper.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
		{Key: "app.identitysignkeeper.stamp", DefaultValue: false, Description: "Fill absent issued_at and, for control or revocation, nonce", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentitySignKeeperOptions)
}
