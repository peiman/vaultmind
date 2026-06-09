package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentityEnrollMetadata defines metadata for the `identity enroll` command.
var IdentityEnrollMetadata = config.CommandMetadata{
	Use:   "enroll",
	Short: "Enroll into a Contract-B network: consume an invite, confirm the fingerprint, self-sign a request",
	Long: `Run the Contract-B MEMBER-onboarding journey. Consume a network INVITE (a
` + "`vmenroll1:`" + ` token or an enroll URL), fetch the relay's
/.well-known/vaultmind-root, CROSS-CHECK it against the invite's trust anchor,
confirm the FINGERPRINT out of band, then assemble + self-sign an enrollment
request via the KEYLESS signer and print the signed request JSON to stdout.

The cross-checks are the security spine: the relay's advertised root MUST decode
to a valid key, be self-consistent (network_id == NetworkID(root)), and match the
invite's anchor (raw bytes AND network id). Any mismatch is a HARD error (wrong
relay / MITM / misconfig) and the flow refuses to proceed.

After signing, the request is SELF-VERIFIED (proof-of-possession). A self-verify
failure means the running signer holds a DIFFERENT key than --pubkey — the flow
fails loudly and emits NOTHING.

--pubkey is your base64-std ed25519 IDENTITY pubkey from ` + "`identity init`" + `.
--transport-pubkey is your base64-std WireGuard pubkey (` + "`wg pubkey`" + `).
Pipe stdout to a file and hand it to your admin out of band; they run
` + "`vaultmind identity enroll-add`" + `. This CLI is KEYLESS: it never opens the key file.`,
	ConfigPrefix: "app.identityenroll",
	FlagOverrides: map[string]string{
		"app.identityenroll.invite":             "invite",
		"app.identityenroll.display_name":       "display-name",
		"app.identityenroll.slug":               "slug",
		"app.identityenroll.pubkey":             "pubkey",
		"app.identityenroll.transport_pubkey":   "transport-pubkey",
		"app.identityenroll.transport_endpoint": "transport-endpoint",
		"app.identityenroll.signer_socket":      "signer-socket",
		"app.identityenroll.yes":                "yes",
	},
}

// IdentityEnrollOptions returns config options for `identity enroll`. Every flag
// description lives here (SSOT) — never inline in cmd/.
func IdentityEnrollOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identityenroll.invite", DefaultValue: "", Description: "Network invite: a vmenroll1: token or enroll URL (required)", Type: "string"},
		{Key: "app.identityenroll.display_name", DefaultValue: "", Description: "Your display name in the network (required; Unicode NFC)", Type: "string"},
		{Key: "app.identityenroll.slug", DefaultValue: "", Description: "Your short ASCII handle/slug (required)", Type: "string"},
		{Key: "app.identityenroll.pubkey", DefaultValue: "", Description: "Your base64-std ed25519 identity pubkey from `identity init` (required)", Type: "string"},
		{Key: "app.identityenroll.transport_pubkey", DefaultValue: "", Description: "Your base64-std 32-byte WireGuard pubkey from `wg pubkey` (required)", Type: "string"},
		{Key: "app.identityenroll.transport_endpoint", DefaultValue: "", Description: "Optional reachable host:port (IPv6 bracketed); omitted when empty", Type: "string"},
		{Key: "app.identityenroll.signer_socket", DefaultValue: "", Description: "Signer socket path (default: XDG state dir)", Type: "string"},
		{Key: "app.identityenroll.yes", DefaultValue: false, Description: "Skip the out-of-band fingerprint confirmation prompt", Type: "bool"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentityEnrollOptions)
}
