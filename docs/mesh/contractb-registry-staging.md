# Contract B — Dev-Interim Registry Staging & Deploy Runbook

**Status: STAGED, NOT DEPLOYED.** This document + the sample registry beside it
(`contractb-registry-staged.sample.json`) are committed-not-deployed artifacts so
live-enablement of the mesh trust root is a config+deploy step with zero crypto
left to invent. Nothing here touches the live channel. The actual deploy +
advisory-verify + the default-deny flip all wait for Peiman's explicit go.

## Decisions baked in (ratified)

- **Root custody (dev-interim):** Mira mints + holds the dev-interim ROOT keypair
  via the slice-2 custody model (a raw 0600 ed25519 key). Single root for v1,
  `root_key_epoch = 0`; root rotation is later. (Door 3 = offline-root,
  dev-interim. Peiman's call, 2026-06-09.)
- **`pubkey = identity`:** the ed25519 public key IS the agent's identity; the
  fingerprint authenticates nothing. The daemon pins the ROOT pubkey and trusts
  the registry the root signs.

## Pinned values (PUBLIC — safe to commit / distribute)

| What | base64-std (RFC 4648 §4, padded) |
|------|-----------------------------------|
| **ROOT pubkey — workhorse's daemon PINS this** | `SWKIlkQh+RWiXAZD8Y0fE/qIGFVoMRpGoaaUSHCvFJ0=` |
| **mira's Contract-B identity pubkey** (key_epoch=1) | `c2tx+bnhb4W/Xcl3sdQzpIyjDK3exYW3oe061c3URsg=` |

Private keys live ONLY in dev-interim custody at
`~/.config/vaultmind/contractb/{root,mira}.key` (0600) and are **never**
committed.

## The staged sample registry

`contractb-registry-staged.sample.json` is a real, root-signed distribution
envelope (`{registry: base64(JCS bytes), root_sig, root_key_epoch}`) carrying
**mira live @ key_epoch=1**. It is verifiable today under the ROOT pubkey above
(loads → resolves mira). It is a **format-and-signing proof, not a deployable
file**, because:

1. **workhorse's binding is absent** (a TODO slot). Minting workhorse's identity
   is the consequential live-enablement custody step — it rides the still-OPEN
   **Door 1** policy (dedicated-service-uid vs single-uid-peercred) and is
   **workhorse's go, Peiman-gated**, same gate as the flip. We do not mint it off
   a chat request.
2. **timestamps are fixed/stale on purpose** (`valid_from = 1780000000`). A live
   registry needs fresh `valid_from`/`valid_until` within the daemon's
   `max_staleness` at deploy time.

## Deploy-time inputs still required (gathered at Peiman's go)

| Input | Owner | Notes |
|-------|-------|-------|
| workhorse's Contract-B pubkey + key_epoch | workhorse | after he mints his identity (his Door-1 custody) |
| fresh `valid_from` / `valid_until` / `max_staleness` | joint | regenerate the registry at deploy so it is fresh |
| `authorized_origin_daemons` ids | joint | the real agent-chat daemon origin id(s); the sample uses the placeholder `agent-chat-dev` |
| `epoch` bump | Mira | monotonic — must exceed the daemon's persisted highest-seen |

## OPEN design fork — surface before deploy

**RESOLVED — option A was built** (`cmd/identity_sign_registry.go`): keyless
`vaultmind identity sign-registry` reads an unsigned registry (stdin or
`--file`), enforces the epoch gate, JCS-canonicalizes, signs via the custody
UDS signer, and prints the distribution envelope. This section previously
described the fork as open after the CLI existed — doc lag, corrected
2026-08-29.

**Live-enablement record (2026-08-28/29):** Peiman gave the go and ran the
root signer himself (dev-interim custody; the CLI stayed keyless). Signed:
epoch 2, mira + workhorse bindings @ key_epoch 1, 30-day window
(1787952703→1790544703), `authorized_origin_daemons: ["daemon:soozani-mesh"]`
per workhorse's proposal (the field is schema-ahead-of-enforcement — nothing
stamps or consumes it yet; when origin enforcement lands, an UNSTAMPED message
against a non-empty list must fail CLOSED, decided then, named now).
Workhorse's Door-1 call: **single-uid-peercred** for dev-interim, residual on
the record: any same-uid process can request signatures from a live signer
until the dedicated-uid tier. Envelope verified against the pinned ROOT
pubkey before handoff; deploy + advisory-verify in progress; the
`contractb_verify` flip remains Peiman's explicit trigger, last.

## Regenerate procedure (deploy time)

The sample was produced by signing a `registry.Registry{...}` (mira binding,
epoch=1) with the root key via `registry.SignRegistry` + `registry.MarshalDistribution`.
At deploy, with workhorse's binding + fresh timestamps:

1. Mint workhorse's identity (his go) → his pubkey.
2. Build `Registry{Epoch: <bump>, ValidFrom: <now>, ValidUntil: <now+window>,
   Agents: [mira, workhorse]}` (both live, `RevokedAt = nil`).
3. Sign with the root key (via the chosen path A or B above) →
   `MarshalDistribution` → the live registry file.
4. Point the daemon at it (`registry_path`) + pin the ROOT pubkey
   (`root_pubkey`), flag `contractb_verify` still **OFF** (advisory).
5. Verify both mira + workhorse emit signed envelopes that verify clean in
   advisory mode on the live channel.
6. **THEN** Peiman flips `contractb_verify = ON` for keyed slugs (default-deny),
   last.

## Sequence (locked)

1. ✅ Trust root + Rust verifier + message-envelope signing — built, merged,
   cross-language byte-exact (registry **and** envelope).
2. ⬅ **HERE:** staged registry sample + pin doc committed (this PR).
3. ⏳ Live-enablement (Peiman's go, joint deploy): workhorse mints his identity →
   regenerate live registry → daemon config → advisory-verify.
4. 🔒 The flip — `contractb_verify = ON`, Peiman's explicit trigger, last.
