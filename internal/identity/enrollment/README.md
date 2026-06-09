# Contract B — Agent Enrollment Request

This package lets an agent **self-sign an enrollment request** so an admin can
**verify proof-of-possession** of the agent's identity key and then decide,
out-of-band, whether to add a binding for that `(slug, pubkey, key_epoch)` to the
trust-root registry. It is the vaultmind half of the agent-enrollment flow; its
Go-signed fixture is the byte-exact artifact workhorse's Rust enrollment daemon
binds to (the 4a→4b pattern, exactly like the registry + envelope fixtures).

It builds strictly on slices 1–4 and reimplements no canonicalization, signing,
or verification.

The deterministic acceptance fixture is
[`testdata/enrollment_request_vectors.json`](testdata/enrollment_request_vectors.json),
kept lockstep with the generator by
`TestEnrollmentRequestFixture_MatchesGenerator`.

## The signed subset

The signature covers the **JCS (RFC 8785) canonical form** of exactly these
fields:

```
JCS({ alg_version, created, display_name, key_epoch, network_id, nonce,
      pubkey, slug, transport_endpoint?, transport_pubkey })
```

`transport_endpoint` is **OPTIONAL**: when absent it is **OMITTED** — not
`null`. In JCS an absent key and a `null` value produce different bytes, so this
distinction is load-bearing across languages.

`pubkey` **IS** in the signed subset **and IS the verification key**. The
self-signature is **proof-of-possession** of that ed25519 identity key — the
whole point of the request.

**Excluded from the signed bytes** (transport): `sig` (the transport-level
signature is the result of signing, never one of the signed bytes).

### Encodings (pinned for cross-language parity)

`pubkey`, `transport_pubkey`, and `sig` are **base64 standard alphabet with
padding** (RFC 4648 §4 — `+`/`/`, `=` padding; *not* URL-safe, *not* unpadded).
The Rust side MUST decode with the standard, padded alphabet or keys and
signatures fail to round-trip.

## Gates (enforced pre-sign, re-surfaced at verify)

All are typed rejects — values are never silently coerced or normalized:

| Field              | Rule                                                                   |
|--------------------|------------------------------------------------------------------------|
| `alg_version`      | must be exactly `1` (anti-downgrade pin)                               |
| `alg_version`, `created` | integers in `[0, 2^53]`                                          |
| `key_epoch`        | integer in `[1, 2^53]` (same anti-rollback floor as the registry)      |
| `display_name`     | valid UTF-8 **and** Unicode NFC (never silently normalized)            |
| `nonce`            | non-empty ASCII (base64 of ≥16 random bytes recommended)               |
| `slug`             | non-empty ASCII                                                        |
| `network_id`       | non-empty (opaque here — the `vmnet1:`-prefixed id is NEVER recomputed) |
| `pubkey`           | base64-std of a 32-byte ed25519 key (wrong-length / small-order rejected) |
| `transport_pubkey` | base64-std of exactly **32 bytes** (Curve25519 — length-only; NOT small-order-checked, it is not an ed25519 key) |

Out-of-range integers are rejected, never silently rounded — JCS renders numbers
as IEEE-754 doubles, so a `created`/`key_epoch` above 2^53 would otherwise lose
precision and break cross-language parity.

`network_id` is treated as an **opaque non-empty string**: this package never
recomputes it from a pubkey. `registry.NetworkID` exists for reference, but the
request carries `network_id` as a string and the gate only checks non-emptiness.

## Verify order

`VerifyEnrollment(fields, sig)`:

1. `CanonicalizeEnrollment(fields)` — re-runs **every** gate over the received
   fields and rebuilds the canonical signed bytes. A gate failure is a typed
   reject.
2. Decode the `pubkey` field to the ed25519 verification key, then ZIP-215
   strict-verify `sig` over the canonical bytes under **that** key.

There is **no registry lookup** — the request is self-contained.

A gate failure or malformed signature returns `(false, non-nil error)`; an honest
signature non-match (wrong key, tampered field) returns `(false, nil)`; a valid
request returns `(true, nil)`.

## Proof-of-possession is NOT authorization

A `(true, nil)` result proves the requester **holds the private key** for the
`pubkey` field. It does **NOT** prove the `slug`/identity is authorized. Adding
the binding to the trust-root registry is the **admin's separate out-of-band
decision** (`identity sign-registry`). This package deliberately performs no
registry lookup and grants no authorization.

## CLI

```
vaultmind identity sign-enrollment [--file req.json] [--signer-socket PATH]
```

Reads the signed-subset enrollment request JSON from stdin (or `--file`),
enforces the gates, canonicalizes, signs the canonical bytes via the **keyless**
signer over its 0600 socket, and prints `{sig, pubkey}` as JSON (the `pubkey` is
echoed — it is the signed proof-of-possession key). The CLI **never opens the
private-key file**; if the signer is unreachable it **fails closed** (an error,
never a silent unsigned result).

## Fixture cases

`testdata/enrollment_request_vectors.json` carries the enrolling agent's signing
pubkey, one **valid** self-signed request (accepted), and reject cases that must
**all** fail: `tampered_display_name`, `wrong_key_self_sig`,
`downgraded_alg_version`, `key_epoch_above_2pow53`, `created_above_2pow53`,
`non_nfc_display_name`, `missing_transport_pubkey`, `bad_transport_pubkey_len`,
`tampered_network_id` (cross-network replay), `empty_nonce`, `non_ascii_nonce`.

Regenerate with:

```
go test ./internal/identity/enrollment/ -run TestGenerate_EnrollmentRequestFixture -update
```
