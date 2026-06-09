// Package invite implements the Contract-B agent-network INVITE token: a
// self-contained, UNSIGNED transport an admin emits so a future
// `identity enroll` can bootstrap trust in a network's root anchor.
//
// # Why unsigned is correct
//
// An invite CARRIES the network's PUBLIC trust anchor (the ed25519 ROOT public
// key) itself, plus where to reach the network (the relay) and the network id
// derived from that key. A signature would have to be verified under some key —
// but the key in question IS the anchor we are trying to establish trust in, so
// a self-signature proves nothing. Authenticity instead comes OUT OF BAND: the
// admin reads the Fingerprint (the vmnet1:… network id) to the member over a
// TRUSTED channel, and `enroll` asks the member to confirm it before trusting
// the anchor. The token is a convenience transport, not a trust root.
//
// # The token + URL format
//
// The bootstrap token is the invitePrefix ("vmenroll1:") followed by the
// base64url-NOPADDING encoding of JSON{ network_id, relay, root_pubkey }. The
// enroll URL is relay + "/enroll#" + the token (the token rides the URL
// fragment, after the '#').
//
// The wire body uses snake_case keys (the FieldNetworkID / FieldRelay /
// FieldRootPubKey SSOT constants); root_pubkey is base64-std (padded) of the
// 32-byte ed25519 root key. The token uses base64url-NOPADDING so it is
// URL/fragment-safe.
//
// # The integrity binding (load-bearing)
//
// network_id MUST equal registry.NetworkID(root_pubkey). Decode RE-DERIVES the
// id from the carried key and rejects any mismatch (ErrNetworkIDMismatch), so a
// relay cannot advertise a victim network's id while substituting its OWN anchor
// key. Every field is also individually gated: the root key is base64-std,
// 32-byte, and small-order-rejected via registry.NewPublicKey; the relay is
// non-empty; the JSON is strict (unknown fields and trailing data rejected).
//
// Decode accepts either a bare token or the full enroll URL (it reads the token
// from the fragment after the first '#'). Encode validates with the SAME rules
// it would accept on Decode, so a token Encode produces always round-trips.
//
// # Reuse
//
// This package reimplements no crypto: it builds on internal/identity/registry
// for NetworkID (the keypair-bound id derivation) and NewPublicKey (the
// wrong-length / small-order validated key constructor).
package invite
