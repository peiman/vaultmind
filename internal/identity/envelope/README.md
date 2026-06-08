# Contract B — Message-Envelope Signing (Slice 5)

This package lets an agent **sign an outgoing chat-message envelope** so a
receiving daemon (workhorse) can **verify the signature + the signer's registry
binding** and then enforce policy. It builds strictly on slices 1–4 and
reimplements no canonicalization, signing, or verification.

This is the contract workhorse's chat-mcp + daemon (and its Rust verifier) bind
to. The deterministic acceptance fixture is
[`testdata/message_signing_vectors.json`](testdata/message_signing_vectors.json),
kept lockstep with the generator by `TestMessageSigningFixture_MatchesGenerator`
(mirroring slice 4's `cross_language_vectors.json`).

## The signed subset

The signature covers the **JCS (RFC 8785) canonical form** of exactly these
fields:

```
JCS({ alg_version, body, from_agent, key_epoch, nonce, room?, seq, to_agent?, ts })
```

where **exactly one** of `room | to_agent` is present and the other is
**OMITTED** — not `null`. In JCS an absent key and a `null` value produce
different bytes, so this distinction is load-bearing across languages.

`from_pubkey` is **DERIVED, NOT SIGNED**: it is never one of the signed bytes.
The verifier resolves the signing pubkey from the registry via
`(from_agent, key_epoch)`. A stamped `from_pubkey` is a convenience hint only and
is never trusted as the verification key.

**Excluded from the signed bytes** (transport / receiver-stamped metadata):
`id`, `sig`, `from_pubkey`, `receive_ts`, `ioguard_verdict`, `origin_daemon`.

## Gates (enforced pre-sign, re-surfaced at verify)

All are typed rejects — values are never silently coerced or normalized:

| Field           | Rule                                                                 |
|-----------------|----------------------------------------------------------------------|
| `alg_version`   | must be exactly `1` (anti-downgrade pin)                              |
| `alg_version`, `seq`, `ts` | integers in `[0, 2^53]`                                  |
| `key_epoch`     | integer in `[1, 2^53]` (same anti-rollback floor as the registry)    |
| `body`          | valid UTF-8 **and** Unicode NFC (never silently normalized)          |
| `nonce`         | non-empty ASCII (base64 of ≥16 random bytes recommended)             |
| `from_agent`    | non-empty (it is the registry resolve key)                           |
| `room`/`to_agent` | exactly one present (both → reject, neither → reject)              |

Out-of-range integers are rejected, never silently rounded — JCS renders numbers
as IEEE-754 doubles, so an epoch/seq/ts above 2^53 would otherwise lose
precision and break cross-language parity.

## Verify order

`VerifyEnvelope(reg, fields, sig, now)`:

1. `CanonicalizeEnvelope(fields)` — re-runs **every** gate over the received
   fields (anti-downgrade, ranges, body NFC, exactly-one routing) and rebuilds
   the canonical signed bytes. A gate failure is a typed reject.
2. `registry.VerifyMessage(reg, from_agent, key_epoch, canonical, sig, now)` —
   resolves the live binding for `(from_agent, key_epoch)`, default-denies a
   revoked / expired / not-yet-valid / epoch-mismatched binding, then ZIP-215
   strict-verifies `sig` over the canonical bytes under the binding's validated
   (small-order-rejected) pubkey.

A gate failure, malformed signature, or registry default-deny returns
`(false, non-nil error)`; an honest signature non-match returns `(false, nil)`; a
valid envelope returns `(true, nil)`.

## Anti-replay is the DAEMON's job — not here

`VerifyEnvelope` authenticates the **signature + the registry binding only**. It
is **stateless**. Anti-replay — the per-`from_agent` `seq` high-water mark and
the `nonce`-unseen set — is the receiving daemon's stateful responsibility
(workhorse). A replayed but otherwise-valid envelope verifies here and **must**
be rejected by the daemon's replay layer. The fixture therefore carries no
"replayed" reject case.

## CLI

```
vaultmind identity sign-envelope [--file env.json] [--signer-socket PATH] [--from-pubkey B64]
```

Reads the signed-subset envelope JSON from stdin (or `--file`), enforces the
gates, canonicalizes, signs the canonical bytes via the **keyless** signer over
its 0600 socket, and prints `{sig, from_pubkey, key_epoch}` as JSON. The CLI
**never opens the private-key file**; if the signer is unreachable it **fails
closed** (an error, never a silent unsigned result). `--from-pubkey` only
populates the `from_pubkey` hint (it is not signed).

## Fixture cases

`testdata/message_signing_vectors.json` carries a pinned root pubkey, a signed
registry with a `mira` binding at `key_epoch=1`, one **valid** signed envelope
(accepted), and reject cases that must **all** fail: `tampered_body`,
`wrong_key_epoch`, `downgraded_alg_version`, `key_epoch_above_2pow53`,
`seq_above_2pow53`, `non_nfc_body`, `both_room_and_to_agent`,
`neither_room_nor_to_agent`, `tampered_sig`, `wrong_signer_key`. (Replay is
daemon-scope and intentionally absent.)

Regenerate with:

```
go test ./internal/identity/envelope/ -run TestGenerate_MessageSigningFixture -update
```
