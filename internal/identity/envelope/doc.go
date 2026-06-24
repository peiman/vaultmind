// Package envelope implements Contract-B SLICE 5: signing and verifying a chat
// MESSAGE envelope. It lets an agent SIGN an outgoing chat-message envelope so a
// receiving daemon (workhorse) can VERIFY the signature + the signer's registry
// binding and then enforce policy.
//
// It builds STRICTLY on slices 1-4 and reimplements nothing:
//
//   - slice 1 (internal/identity): identity.Canonicalize (RFC 8785 JCS) for the
//     canonical signed bytes; identity.ValidateSchema for the shared
//     integer-range / UTF-8 / NFC gate; identity.SignCanonical / VerifyCanonical
//     for the low-level ed25519 (small-order-rejecting, cofactorless strict
//     verify) primitives.
//   - slice 2 (internal/identity/signer): the KEYLESS UDS custody signer — the
//     CLI signs via the signer process over its 0600 socket and NEVER opens the
//     private-key file. SignEnvelope takes a SignerClient seam, not a key.
//   - slice 3 (internal/identity/registry): registry.VerifyMessage resolves the
//     signer's binding by (from_agent slug, key_epoch) -> validated pubkey, then
//     VerifyCanonical. VerifyEnvelope delegates the binding+signature check to it.
//   - slice 4: the cross_language_vectors.json fixture+generator pattern, mirrored
//     here as testdata/message_signing_vectors.json so workhorse's Rust verifier
//     binds to the SAME byte-for-byte acceptance contract.
//
// # The FROZEN signing contract (agreed with workhorse — do NOT deviate)
//
// The signature covers the JCS canonical form of the SIGNED SUBSET:
//
//	JCS({ alg_version, body, from_agent, key_epoch, nonce, room?, seq, to_agent?, ts })
//
// where EXACTLY ONE of room | to_agent is present and the other is OMITTED (not
// null — in JCS an absent key and a null value are different bytes, so this is
// load-bearing). The signed-subset field names are the Field*/SignedSubset SSOT
// constants in envelope.go.
//
// Pre-sign gates (enforced by CanonicalizeEnvelope, surfaced again at verify):
//
//   - alg_version, key_epoch, seq, ts are integers in [0, 2^53]; key_epoch is
//     additionally in [1, 2^53] (same epoch rule as the registry). Out-of-range
//     is a TYPED reject — never a silent IEEE-754 round (JCS renders numbers as
//     doubles).
//   - alg_version MUST be exactly AlgVersion (1) — an anti-downgrade pin.
//   - body MUST be valid UTF-8 and Unicode NFC; a non-NFC / invalid-UTF8 body is
//     rejected, never silently normalized.
//   - nonce MUST be ASCII (base64 of >= 16 random bytes recommended).
//   - EXACTLY ONE of room | to_agent present (both -> reject, neither -> reject).
//
// from_pubkey is DERIVED, NOT SIGNED: it is NOT one of the signed bytes. The
// verifier resolves the signing pubkey from the registry via (from_agent,
// key_epoch); a stamped from_pubkey is a convenience/hint only and is never
// trusted as the verification key.
//
// EXCLUDED from the signed bytes (transport / receiver-stamped metadata): id,
// sig, from_pubkey, receive_ts, ioguard_verdict, origin_daemon.
//
// # Verify order
//
// VerifyEnvelope:
//  1. CanonicalizeEnvelope(fields) — re-run EVERY pre-sign gate over the
//     received fields (anti-downgrade, ranges, body NFC, exactly-one routing) and
//     rebuild the canonical signed bytes. A gate failure is a typed reject.
//  2. registry.VerifyMessage(reg, from_agent, key_epoch, canonical, sig, now) —
//     resolve the live binding for (from_agent, key_epoch), default-deny a
//     revoked/expired/not-yet-valid/epoch-mismatched binding, then
//     cofactorless-strict-verify sig over the canonical bytes under the
//     binding's validated pubkey.
//
// # Boundary: anti-replay is the DAEMON's job, NOT here
//
// VerifyEnvelope authenticates the SIGNATURE + the registry BINDING only. It is
// STATELESS. Anti-replay — the per-(from_agent) seq high-water mark and the
// nonce-unseen set — is the receiving daemon's stateful responsibility
// (workhorse). This package deliberately keeps no replay state; a replayed but
// otherwise-valid envelope verifies here and MUST be rejected by the daemon's
// replay layer. The fixture therefore carries no "replayed" reject case.
package envelope
