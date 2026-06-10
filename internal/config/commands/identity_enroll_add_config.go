package commands

import "github.com/peiman/vaultmind/.ckeletin/pkg/config"

// IdentityEnrollAddMetadata defines metadata for the `identity enroll-add`
// command — the ADMIN counterpart to `identity enroll`.
var IdentityEnrollAddMetadata = config.CommandMetadata{
	Use:   "enroll-add",
	Short: "Admin-add a member's enrollment request to the trust-root registry (Contract-B)",
	Long: `Run the Contract-B ADMIN side of onboarding. Read a member's signed
enrollment-request JSON (the output of ` + "`identity enroll`" + `), VERIFY
proof-of-possession, confirm the request is for YOUR network, add the member's
binding to the trust-root REGISTRY, and print the UPDATED UNSIGNED registry to
stdout for ` + "`identity sign-registry`" + ` to root-sign.

This verb does NOT sign — the established two-verb split (build-then-sign):

  enroll > request.json
    -> enroll-add --request request.json --registry current.json > new-unsigned.json
    -> sign-registry --file new-unsigned.json > new-signed.json   (ROOT signer)

--registry accepts either an UNSIGNED wireRegistry or a SIGNED distribution
envelope (auto-detected); absent => a fresh registry (first emit = epoch 1). A
signed-envelope --registry is INTEGRITY-verified against --root-pubkey before any
mutation (the root signature must hold — freshness/rollback are NOT checked, so
an admin can mutate a stale registry) and is REFUSED without --root-pubkey.

At least one of --root-pubkey or --network-id is required to resolve the admin
network; when both are set they must AGREE. A request whose network_id is not the
admin network is REFUSED (cross-network). Uniqueness is checked FAIL-CLOSED before
the append: a duplicate {slug,key_epoch} or a slug that already has a live binding
is refused (a bad append would poison the whole registry at the consumer).

NOTE: transport_pubkey/transport_endpoint are NOT yet carried into the binding
(WireGuard wiring is a later slice). This CLI is KEYLESS: it never signs.`,
	ConfigPrefix: "app.identityenrolladd",
	FlagOverrides: map[string]string{
		"app.identityenrolladd.request":          "request",
		"app.identityenrolladd.registry":         "registry",
		"app.identityenrolladd.root_pubkey":      "root-pubkey",
		"app.identityenrolladd.network_id":       "network-id",
		"app.identityenrolladd.validity_seconds": "validity-seconds",
		"app.identityenrolladd.origin_daemon":    "origin-daemon",
	},
}

// IdentityEnrollAddOptions returns config options for `identity enroll-add`.
// Every flag description lives here (SSOT) — never inline in cmd/.
func IdentityEnrollAddOptions() []config.ConfigOption {
	return []config.ConfigOption{
		{Key: "app.identityenrolladd.request", DefaultValue: "", Description: "Signed enrollment-request JSON file (stdin when empty or \"-\")", Type: "string"},
		{Key: "app.identityenrolladd.registry", DefaultValue: "", Description: "Current registry file: unsigned wireRegistry OR signed envelope (absent => fresh)", Type: "string"},
		{Key: "app.identityenrolladd.root_pubkey", DefaultValue: "", Description: "Base64-std root ed25519 pubkey (required for a signed-envelope --registry; derives the network)", Type: "string"},
		{Key: "app.identityenrolladd.network_id", DefaultValue: "", Description: "Admin network id (vmnet1:…); alternative to --root-pubkey (>=1 required; both must agree)", Type: "string"},
		{Key: "app.identityenrolladd.validity_seconds", DefaultValue: "", Description: "Registry+binding issuance window in seconds (default 31536000 = one year)", Type: "string"},
		{Key: "app.identityenrolladd.origin_daemon", DefaultValue: "", Description: "Comma-separated authorized origin daemon ids for the new binding (default none)", Type: "string"},
	}
}

func init() {
	config.RegisterOptionsProvider(IdentityEnrollAddOptions)
}
