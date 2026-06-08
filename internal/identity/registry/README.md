# Contract B — Registry Distribution Envelope (Slice 4)

This package implements the Contract-B **trust-root registry** (slice 3) and its
**`--registry` distribution envelope** (slice 4): the on-disk / over-wire format
a root-signed `SignedRegistry` is serialized to, plus the deterministic
cross-language acceptance fixture that workhorse's Rust verifier binds to.

The distribution layer is **(de)serialization + fail-closed parsing only**. All
trust logic (root-sig verify, anti-rollback, freshness, revocation, resolve,
message verify) lives in `registry.go` (slice 3) and is **not** duplicated here.

## Distribution envelope JSON

A distributed `SignedRegistry` is a single JSON object:

```json
{
  "registry": "<base64-std of the JCS-canonical registry bytes>",
  "root_sig": "<base64-std of the 64-byte ed25519 root signature>",
  "root_key_epoch": 0
}
```

- `registry` — base64-std (RFC 4648, padded) of the **exact** JCS-canonical
  registry bytes that `root_sig` covers. These bytes are emitted **verbatim**;
  they are never re-canonicalized or double-JSON-encoded. The decoded payload is
  raw JCS JSON (begins with `{`), not a quoted string.
- `root_sig` — base64-std of the ed25519 signature by the offline root key over
  the decoded `registry` bytes. Must decode to exactly `ed25519.SignatureSize`
  (64) bytes.
- `root_key_epoch` — a bare JSON integer identifying which root key signed (for
  root rotation).

`MarshalDistribution(SignedRegistry) ([]byte, error)` produces this JSON;
`ParseDistribution([]byte) (SignedRegistry, error)` parses it.

### Fail-closed parsing

`ParseDistribution` returns an error **and a zero `SignedRegistry`** (never a
partial value that could be mistaken for valid) on any malformed input:

- non-JSON / malformed JSON,
- an unknown/extra field (strict decoding — no silent drop of a smuggled key),
- a missing or empty `registry` or `root_sig`,
- a `registry` or `root_sig` that is not valid base64-std,
- a `root_sig` that does not decode to 64 bytes.

It never panics. It does **not** verify the root signature, freshness, or
anti-rollback — those are `VerifyAndLoad`'s job.

## JCS canonical registry / binding shape

The registry bytes inside the envelope are the RFC 8785 (JCS) canonical form of:

```jsonc
{
  "agents": [
    {
      "authorized_origin_daemons": ["daemon-1"],
      "display_name": "Mira ⭐",     // raw UTF-8, never \u-escaped
      "key_epoch": 1,                // bare integer
      "pubkey": "<base64-std 32-byte ed25519 pubkey>",
      "revoked_at": 1700000000,      // OMITTED when the binding is live
      "slug": "mira",
      "valid_from": 1990000,         // unix seconds, bare integer
      "valid_until": 3000000
    }
  ],
  "epoch": 10,                       // monotonic anti-rollback counter, bare int
  "valid_from": 1990000,
  "valid_until": 3000000
}
```

Canonicalization rules (enforced by `identity.Canonicalize`):

- object keys sorted lexicographically by UTF-16 code unit,
- field names `snake_case`,
- pubkeys base64-std,
- integers rendered bare (no decimal point / exponent); epochs are bounded to
  `[1, 2^53]` so they round-trip across languages without IEEE-754 precision
  loss,
- `revoked_at` **omitted** when the binding is live (nil pointer == live),
  present only when revoked,
- string values preserved as raw UTF-8 (e.g. `⭐` stays `0xE2 0xAD 0x90`).

## Verify order (consumer / Rust verifier contract)

`ParseDistribution` then `registry.VerifyAndLoad` perform, **fail-closed at every
step, in this order**:

1. **Parse** the envelope (fail-closed on malformed input, above).
2. **Root signature first**, over the **received** registry bytes — verify
   `root_sig` against the **pinned** root pubkey using ZIP-215 strict
   verification with an explicit small-order pubkey reject (cofactor check). A
   tampered body or a small-order root key is rejected **before** the body is
   trusted.
3. **Decode** the now-authenticated bytes; every agent pubkey is re-validated
   (wrong-length / small-order / undecodable agent keys are rejected at load).
4. **Epoch range** — reject a registry epoch or any binding `key_epoch` outside
   `[1, 2^53]`.
5. **Uniqueness** — reject duplicate `{slug, key_epoch}` tuples or more than one
   live binding per slug (shadowing defense).
6. **Anti-rollback** — reject `epoch <= persistedHighestEpoch`.
7. **Freshness (fail closed)** — reject if `now < valid_from`
   (**reject-future-`valid_from`**), `now > valid_until`, or
   `now - valid_from > maxStaleness`. A stale registry may hide a revocation.

Then, per message:

8. **Resolve** the live binding for the slug at `now`, default-denying a
   **revoked**, **expired**, or **not-yet-valid** (`now < binding.valid_from`)
   binding.
9. **VerifyMessage** — require the caller's `key_epoch` to match the resolved
   binding (default-deny on mismatch), then verify the message signature under
   the binding's pubkey with the same ZIP-215 strict + small-order +
   non-canonical-S rejection.

## Cross-language acceptance fixture

`testdata/cross_language_vectors.json` is the **gating acceptance test** and the
stable artifact workhorse's Rust verifier binds to. It is generated
deterministically (fixed ed25519 seeds) by `TestGenerate_CrossLanguageFixture`
(run `go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageFixture -update`)
and committed.

Top-level shape:

- `root_pubkey` — base64-std of the pinned root pubkey to verify against.
- `now`, `max_staleness_secs` — the reference time and staleness bound the
  negatives are relative to.
- `valid` — a complete `--registry` envelope that loads under
  `root_pubkey`/`now`/`max_staleness_secs`, plus the expected resolved binding
  (`expect_slug` / `expect_key_epoch` / `expect_pubkey`) and a sample
  `{slug, key_epoch, canonical_message_bytes, signature}` that `VerifyMessage`
  **accepts**.
- `reject` — named negative cases, each `{name, reason, expect:"reject",
  envelope, ...}` with optional `root_pubkey` / `now` / `persisted_highest_epoch`
  overrides and an optional message sample. Cases cover: small-order root pubkey,
  small-order agent pubkey, non-canonical S in the message signature,
  future `valid_from` (registry-level and binding-level), rollback
  (`epoch <= persisted`), stale (`now > valid_until` and beyond `max_staleness`),
  revoked binding, and a tampered registry body.

The Go side proves the fixture is correct **before** Rust consumes it:
`TestCrossLanguageFixture_ValidLoadsAndVerifies` asserts the valid case
loads + resolves + verifies, and `TestCrossLanguageFixture_AllRejectsAreRejected`
asserts **every** reject case is rejected somewhere in
`ParseDistribution` / `VerifyAndLoad` / `Resolve` / `VerifyMessage`. The Rust
`verify_strict` + cofactor + freshness + anti-rollback path must reject the same
cases and accept the same valid case.
