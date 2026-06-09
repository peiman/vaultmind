// Package enrollment implements the Contract-B AGENT ENROLLMENT REQUEST: an
// agent SELF-SIGNS a request so an admin can VERIFY proof-of-possession of the
// agent's identity key and then decide, out-of-band, whether to add a binding
// for that (slug, pubkey, key_epoch) to the trust-root registry. It is the
// vaultmind half of the agent-enrollment flow; its Go-signed fixture is the
// byte-exact artifact workhorse's Rust enrollment daemon binds to (the 4a->4b
// pattern, exactly like the registry + envelope fixtures).
//
// It builds STRICTLY on slices 1-4 and reimplements nothing:
//
//   - slice 1 (internal/identity): identity.Canonicalize (RFC 8785 JCS) for the
//     canonical signed bytes; identity.SignCanonical / VerifyCanonical for the
//     low-level ed25519 (small-order-rejecting, ZIP-215) primitives.
//   - slice 2 (internal/identity/signer): the KEYLESS UDS custody signer — the
//     CLI signs via the signer process over its 0600 socket and NEVER opens the
//     private-key file. SignEnrollment takes a SignerClient seam, not a key.
//   - slice 3 (internal/identity/registry): registry.NewPublicKey validates the
//     pubkey field (32-byte ed25519, small-order-rejected) so a hostile key
//     cannot enter the signed subset.
//   - slice 4/5: the cross-language fixture+generator pattern, mirrored here as
//     testdata/enrollment_request_vectors.json so workhorse's Rust verifier binds
//     to the SAME byte-for-byte acceptance contract.
//
// # The FROZEN signing contract (agreed with workhorse — do NOT deviate)
//
// The signature covers the JCS canonical form of the SIGNED SUBSET:
//
//	JCS({ alg_version, created, display_name, key_epoch, network_id, nonce,
//	      pubkey, slug, transport_endpoint?, transport_pubkey })
//
// where transport_endpoint is OPTIONAL and, when absent, is OMITTED (not null —
// in JCS an absent key and a null value are different bytes, so this is
// load-bearing). The signed-subset field names are the Field* SSOT constants in
// enrollment.go.
//
// pubkey IS in the signed subset AND IS the verification key: the self-signature
// is PROOF-OF-POSSESSION of the agent's ed25519 identity key. transport_pubkey is
// a 32-byte Curve25519 WireGuard key (REQUIRED, signed).
//
// Pre-sign gates (enforced by CanonicalizeEnrollment, surfaced again at verify):
//
//   - alg_version, created are integers in [0, 2^53]; key_epoch is in [1, 2^53]
//     (same epoch rule as the registry). Out-of-range is a TYPED reject — never a
//     silent IEEE-754 round (JCS renders numbers as doubles).
//   - alg_version MUST be exactly AlgVersion (1) — an anti-downgrade pin.
//   - display_name MUST be valid UTF-8 and Unicode NFC; a non-NFC / invalid-UTF8
//     value is rejected, never silently normalized.
//   - nonce MUST be non-empty ASCII (base64 of >= 16 random bytes recommended).
//   - slug MUST be non-empty ASCII.
//   - network_id MUST be non-empty. It is treated as an OPAQUE string here and is
//     NEVER recomputed (registry.NetworkID exists for reference only).
//   - pubkey MUST be base64-std of a 32-byte ed25519 key (wrong-length /
//     small-order rejected via registry.NewPublicKey).
//   - transport_pubkey MUST be base64-std of exactly 32 bytes (Curve25519 —
//     length-only checked; it is NOT an ed25519 key, so it is NOT
//     small-order-checked).
//
// EXCLUDED from the signed bytes (transport): sig (the transport-level signature
// is the RESULT of signing, never one of the signed bytes).
//
// # Verify order
//
// VerifyEnrollment:
//  1. CanonicalizeEnrollment(fields) — re-run EVERY pre-sign gate over the
//     received fields and rebuild the canonical signed bytes. A gate failure is a
//     typed reject.
//  2. Decode the pubkey field to the ed25519 verification key, then ZIP-215
//     strict-verify sig over the canonical bytes under THAT key.
//
// There is NO registry lookup — the request is self-contained.
//
// # Proof-of-possession is NOT authorization
//
// A (true, nil) result proves the requester HOLDS the private key for the pubkey
// field. It does NOT prove the slug/identity is authorized — adding the binding
// to the trust-root registry is the ADMIN's separate out-of-band decision
// (identity sign-registry). This package performs no registry lookup and grants
// no authorization.
package enrollment
