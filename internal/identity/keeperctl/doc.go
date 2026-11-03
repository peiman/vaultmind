// Package keeperctl defines the frozen keeper control envelope and a reference
// watchdog verifier. It reuses Contract B's identity.Canonicalize (RFC 8785 JCS),
// identity.ValidateSchema and registry.VerifyMessage, including Contract B's
// cofactorless strict Ed25519 verification. It holds no owner private keys:
// SignControlRequest, SignGrant and SignGrantRevoke use the keyless SignerClient
// seam, compatible with the existing custody signer.
//
// # Signed documents
//
// Every document signs alg_version=1, kind, from, key_epoch and issued_at.
// keeper.control additionally signs keeper, action, nonce, optional body and
// optional grant_id. keeper.grant additionally signs grant_id, grantee, keepers,
// scopes and expires_at. keeper.grant_revoke additionally signs grant_id and
// nonce. Field* constants are the signed-name SSOT. No signature or public-key
// hint belongs in these documents; signatures travel separately. Kind prevents
// a signature from being used as another form of instruction.
//
// All integers are in [0, 2^53], with key_epoch in [1, 2^53]. Parsing rejects
// float, exponent and string representations of integers, unknown fields,
// duplicate keys, and nulls. Optional body and grant_id are pointers: absence
// omits the key, never signs null. All strings must already be UTF-8 and NFC;
// they are rejected rather than normalized. keeper, from, grant_id, grantee and
// every keepers element are IDs: printable ASCII, 1..128 bytes. Nonces are
// printable ASCII, 16..128 bytes. message and rebrief require a nonempty body of at most
// 65536 UTF-8 bytes. All other actions forbid body. Only reads allow grant_id. Grant arrays
// are nonempty and strictly byte-ascending, never silently sorted or deduplicated.
// A grant expires strictly after its issue time. Gate and Reason are frozen
// machine-readable vocabularies; Detail is diagnostic and is not frozen.
//
// Parse errors use the same GateError vocabulary: input that is not exactly one
// JSON object is malformed_json, duplicate keys are unknown_field, a value of the
// wrong JSON type is wrong_type, and any explicit null is null_field. Missing body is body_required
// for the actions that require it. Gate precedence for inputs violating several
// constraints is pinned by the vectors where present.
//
// # Delivery and revocation
//
// The owner delivers a signed keeper.grant document plus its detached signature
// to the watchdog. AddGrant verifies the document and every keeper's ownership
// before installing both fields and signature. A grantee subsequently sends a
// signed read request carrying only grant_id, not a copy of the grant. The owner
// delivers a signed keeper.grant_revoke document with its detached signature;
// RevokeGrant verifies freshness, start floor, signature, nonce and original
// issuer before permanently marking that grant ID revoked. Revocation cannot
// be undone by reinstallation. Expired grants and their revocation records may
// be pruned together; AddGrant refuses expired grants. IDs cannot be overwritten
// with different signed content. Re-delivery of the identical live grant is
// idempotent. An owner's valid read ignores grant_id: the owner needs no grant.
//
// # Verification order
//
// VerifyControl fails closed, in this order:
//  1. All schema gates and canonicalization.
//  2. The registry-level window (both endpoints inclusive).
//  3. Registry tuple resolution and signature verification.
//  4. Keeper presence in the watchdog's own ownership manifest.
//  5. Watchdog start floor, maximum age, then maximum future skew.
//  6. Atomic nonce check-and-record, before authorization.
//  7. Owner acceptance, owner-only control enforcement, or delegated read checks.
//
// Tuple/verification errors, including structurally malformed or empty
// signatures, are binding_rejected, exactly as registry.VerifyMessage reports
// them. A well-formed nonmatching signature is signature_invalid. A missing or
// failed nonce store is nonce_store_failed. A failed grant lookup is grant_missing.
// Grant checks run in this order: presence, revocation, grantee, keeper, expiry,
// scope, then live signature verification against the CURRENT keeper owner.
// Rotation or revocation of the owner's signing tuple invalidates old grants.
// Grant expiry is exclusive: now >= expires_at refuses a read.
//
// Policy.Registry must already have been authenticated by the trust root (for
// example via registry.VerifyAndLoad); keeperctl does not authenticate a supplied
// ownership manifest or registry. Policy.NotBefore must be positive and is the
// watchdog's start time, so restarting with a fresh in-memory nonce store cannot
// reopen earlier request windows. A missing floor or negative freshness window
// fails at the time-check stage with schema_invalid. Defaults are MaxAge=300s and
// MaxSkew=30s. The window is computed in whole epoch seconds (now truncated to
// the second), so an integer-clock verifier reaches the same verdict. The age
// and skew boundaries and NotBefore floor accept equality.
// Configure MemoryNonceStore with the same or longer retention windows; it
// retains entries through issued_at + MaxAge + MaxSkew and lazily prunes only
// after that instant. No file nonce store is necessary.
//
// MemoryGrantStore is safe for concurrent calls and never prunes. FileGrantStore
// uses a single versioned JSON document containing signed grants and revoked IDs
// with their expiration times. It validates corrupt state before any pruning,
// writes mode-0600 temporary files in the same directory, fsyncs, renames, and
// fsyncs the directory. Each successful write publishes a complete snapshot;
// write errors are returned and poison that handle until reopen reconciles the
// durable snapshot, including an uncertain rename/fsync result. Use one open store
// per path in an owner-controlled directory; serialize multiple processes
// externally. Expiry pruning happens on Open and Put, never before expires_at.
//
// # Cross-language contract
//
// testdata/keeper_control_vectors.json contains deterministic signed forms,
// canonical UTF-8 strings, SHA-256 hex, public-key hex, base64 signatures, schema
// rejections, and complete reference-verifier scenarios. Fixed Ed25519 seeds are
// 32 bytes filled with 0x01 (owner), 0x02 (reader), 0x03 (stranger), or 0x04
// (rotated owner). The fixture
// registry is already trusted input, not a registry distribution envelope.
// nonce_store_failure means the scenario's nonce store returns an I/O failure.
// Prior requests seed the nonce store before verification. Accepted decisions
// have a role and no reason; refusals have a reason and no role. Regenerate only
// deliberately with:
//
//	go test ./internal/identity/keeperctl/ -run TestGenerateKeeperControlFixture -update
//
// The match test enforces byte-for-byte agreement with the generator; acceptance
// tests load the file and replay its inputs independently of the generator.
//
// Administrative stop by the operating system (an OS kill) is outside this
// verifier by design. This contract authorizes signed keeper instructions; it
// neither intercepts nor restricts the machine administrator's process control.
package keeperctl
