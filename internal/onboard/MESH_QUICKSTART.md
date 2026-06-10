# VaultMind — Mesh Onboarding (Contract-B)

How the `identity` verbs fit together to join a Contract-B agent network: the
admin runs a one-time network setup; each member mints a key and enrolls against
the network root. Run `vaultmind doctor` to confirm you're live. Full design +
deploy runbook: docs/mesh/contractb-registry-staging.md.

Two honest properties hold throughout: the CLI is **keyless** — your ed25519
private key lives in a separate `signer` process reached over a 0600 socket; the
CLI never opens it. And trust is bootstrapped **out-of-band**: you confirm the
network's root fingerprint over a channel you already trust, not over the relay.

## Admin — one-time network setup

1. Run the ROOT signer (custody of the network root key). DEV-INTERIM posture;
   production = air-gapped / hardware-held.

   ```bash
   vaultmind identity signer --signer-key <root.key>
   ```

2. Emit an invite for a new member (carries the root pubkey + relay + the
   out-of-band fingerprint the member reads to confirm the root).

   ```bash
   vaultmind identity invite --root-pubkey <root-pub-b64> --relay <relay-url>
   ```

3. The member enrolls and hands you their signed request out-of-band — v1 has
   no automated submit; the request reaches you on a channel you trust.

4. Add the member's request to the registry (emits the updated UNSIGNED registry).

   ```bash
   vaultmind identity enroll-add --request <member.json> --registry <current.json> --root-pubkey <root-pub-b64> > updated.json
   ```

5. Root-sign the updated registry — the human-gated step (the root signer must
   be running).

   ```bash
   vaultmind identity sign-registry --file updated.json > signed.json
   ```

6. Serve `signed.json` from the relay at `/.well-known/vaultmind-directory`.

## Member — join a network

1. Mint your identity key (seals the private key to the signer, prints your
   public key).

   ```bash
   vaultmind identity init
   ```

2. Run your signer (holds your key; the CLI stays keyless).

   ```bash
   vaultmind identity signer
   ```

3. Enroll from the invite your admin gave you: cross-check the relay's root
   against the invite, confirm the fingerprint OUT-OF-BAND, and self-sign your
   enrollment request. (The admin's invite pins the network root locally.)

   ```bash
   vaultmind identity enroll --invite <vmenroll1:...> --display-name "..." --slug <you> --pubkey <your-pub-b64> --transport-pubkey <wg-pub-b64>
   ```

4. Hand the printed request to your admin out-of-band (v1 is OOB-to-admin; there
   is no automated submission).

5. Once they've added + root-signed you, confirm you're live.

   ```bash
   vaultmind doctor
   ```

## Am I live?

`vaultmind doctor` (mesh section): you are authenticated when your binding
resolves in the served registry against your PINNED root AND your signer proves
it holds your key. NOTE: doctor checks your IDENTITY liveness; whether your chat
client is subscribed and delivering is the agent-chat side, not this check.
