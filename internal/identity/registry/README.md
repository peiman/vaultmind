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
  "root_key_epoch": 0,
  "root_sig": "<base64-std of the 64-byte ed25519 root signature>"
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
- trailing bytes after the envelope object (concatenated-object or garbage defense),
- a missing or empty `registry` or `root_sig`,
- a `registry` or `root_sig` that is not valid base64-std,
- a `root_sig` that does not decode to 64 bytes,
- a negative `root_key_epoch` (key epochs are non-negative; a negative value is structurally invalid).

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

## Agent roles

A binding may carry `roles`, the capabilities the root grants that agent.
Three kinds are recognized:

```json
"roles": [
  {"kind": "bridge", "vouches_for": ["human:peiman"]},
  {"kind": "code_signing", "platforms": ["darwin-arm64"]},
  {"kind": "admin"}
]
```

- `bridge` — `vouches_for`, the principals this agent may vouch for.
- `code_signing` — `platforms`, the platforms this agent may sign for.
- `admin` — no parameter list. Any field besides `kind` rejects the registry.

An empty `vouches_for` or `platforms` list is valid and grants nothing. An
unknown `kind`, a missing variant field, an extra field on a role, or a
`roles` value that is not an array rejects the whole registry. Unknown fields
elsewhere on the registry or on a binding stay ignored on verify. Unknown
fields inside a role object do not.

`roles` is **omitted** from the canonical bytes when the list is empty. That
keeps a roles-free registry byte-identical to one signed before the field
existed, so existing signatures and `testdata/cross_language_vectors.json`
stay valid. Role objects are JCS-canonicalized with the rest of the registry
(keys sorted). The array order is the order that was signed.

`testdata/cross_language_roles_vectors.json` is the acceptance fixture for
this field: one valid envelope (a `bridge` role, a `code_signing` role, and a
binding with no roles) plus root-signed schema rejects. It is generated by
`TestGenerate_CrossLanguageRolesFixture` (`-update`). It is a separate file
because a verifier that predates roles rejects a registry that carries them.
Do not sign roles until every verifier that must accept the registry parses them.

## Rooms, DMs, and the admin role

A registry may carry room policy and a DM policy beside the roles above.
`sign-registry` and `VerifyAndLoad` share one parser. Any violation rejects
the whole registry, on the unsigned sign input and on a root-signed document.

```json
{"rooms": [{"name": "product-control",
            "controller": "human:peiman",
            "members": [{"principal": "agent:workhorse", "access": ["read", "post"]}],
            "approval_required": ["merge", "publish"]}],
 "dms": "allow"}
```

`rooms` is an array of at most 256 objects. Absent means no rooms. An empty
array is accepted and then omitted from the canonical bytes, the same as
absent. Each room object:

- `name` — 1–128 bytes, ASCII. The first byte is alphanumeric. Every byte is
  in `[A-Za-z0-9._-:/]`. The name must not end with the bytes `.control`
  (that check is case-sensitive, so `Room.Control` is a different name).
  Names are unique byte for byte, and no two names may be equal when ASCII
  `A-Z` is folded to `a-z`.
- `controller` — required. An identity `kind:name`, kind `agent` or `human`
  only, at most 128 bytes in total. `kind` is 1–32 bytes, starts with a
  lowercase letter, and uses `[a-z0-9-]`. `name` is non-empty, starts with an
  alphanumeric byte, and uses `[A-Za-z0-9._-]`.
- `members` — optional, at most 256. Each member is exactly
  `{principal, access}`. `principal` follows the controller rule, is unique
  in the room by byte comparison, and is not the room's controller. An empty
  list is omitted from the canonical bytes.
- `access` — a non-empty array. Each entry is exactly `"read"` or `"post"`,
  with no repeats. The array is signed in the order given.
- `approval_required` — optional, at most 32, no repeats. Each name matches
  `^[a-z][a-z0-9_-]{0,31}$`. The array is signed in the order given. An empty
  list is omitted from the canonical bytes.

An unknown field on a room or a member is `registry: malformed room`.

`dms` is an optional string, exactly `"allow"` or `"agents_off"`. Absent
means allow, and the key stays absent in the signed bytes: the default is
not written in. An explicit `"allow"` is signed. Anything else, including
JSON null, is `registry: malformed DM policy`.

`rooms` and `dms` are emitted only when present. A registry with neither
canonicalizes to the same bytes as one signed before these fields existed, so
`testdata/cross_language_vectors.json` and
`testdata/cross_language_roles_vectors.json` stay valid.
`testdata/cross_language_rooms_vectors.json` is the acceptance fixture for
this shape: one valid envelope (two rooms, `dms` set to `agents_off`, and an
`admin` binding) plus one root-signed reject per rule. It is generated by
`TestGenerate_CrossLanguageRoomsFixture` (`-update`). A verifier that predates
rooms, DM policy, or the admin role rejects a registry that carries them. Do
not sign them until every verifier that must accept the registry parses them.
