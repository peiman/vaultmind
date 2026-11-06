# Hub session proof

Normative contract for `vaultmind identity sign-session`. The agent signs. The hub verifies. This document is the signer side; the hub's verifier is a separate program and must accept these bytes.

## Signed bytes

The signature covers the UTF-8 domain tag immediately followed by the JCS (RFC 8785) canonical bytes of the statement. The tag is not inside the JSON.

```text
vaultmind-hub-session-v1:
```

```text
<tag><JCS(statement)>
```

JCS sorts the keys. For this object the order is `challenge`, `key_epoch`, `network_id`, `purpose`, `slug`, `ts`.

## Statement

| Field | Rule |
| --- | --- |
| `challenge` | Standard base64 (RFC 4648 §4, with padding). Decodes to 16–64 bytes. Stored exactly as the hub issued it. Not re-encoded. |
| `key_epoch` | Integer in `[1, 2^53]`, inclusive. The signer's own registry epoch. |
| `network_id` | The string the hub issued with the challenge. Non-empty, at most 128 bytes, valid UTF-8, NFC, and no ASCII control (U+0000–U+001F or U+007F). Rejected when it is not. Never normalized. |
| `purpose` | Exactly `hub-session`. |
| `slug` | The signer's own bare registry slug. Non-empty, valid UTF-8, NFC, and no ASCII control (U+0000–U+001F or U+007F). Rejected when it is not. Never normalized. |
| `ts` | Unix seconds in `[0, 2^53]`, inclusive, taken from the signer's clock. `2^53` is the JCS-safe ceiling, not a freshness window. |

`slug` and `key_epoch` are resolved from this machine's enrollment: the `agents.yaml` entry whose `project_path` matches this project, then the live binding `registry.Resolve` returns under the pinned root. The command has no `--slug`, `--key-epoch`, or `--mesh-slug`. `app.doctor.mesh_slug` is not read. A caller cannot name another agent.

The signature is ed25519 over those tagged bytes, checked with cofactorless strict verification (dalek `verify_strict`). The command prints one JSON line and nothing else:

```json
{"statement":{...},"sig":"<base64>","pubkey":"<base64>"}
```

`sig` is the signature. `pubkey` is the resolved binding's public key. Both are standard base64. Any gate failure, missing enrollment, signer error, or signature that does not verify under that pubkey writes nothing and exits non-zero.

## Why this domain is separate

An envelope signature and a registry signature cover untagged JCS. A doctor self-verify covers `vaultmind-doctor-selfverify-v1:` plus a nonce. A session proof covers `vaultmind-hub-session-v1:` plus the JCS statement. A correct signature from any of those other domains does not verify as a session proof, and a session proof does not verify as any of them. There is no envelope prefix to strip: the envelope case and a signature with no tag are the same bytes.

The Go reference is `identity.VerifySessionStatement`. It rebuilds the tagged bytes from the statement and verifies. It does not check single-use, network identity, or the time window.

Cross-language vectors: `internal/identity/testdata/session_statement_vectors.json`. One valid statement (canonical JSON hex, tagged bytes hex, signature, pubkey) and these rejects, each a correct signature over the wrong bytes:

- `wrong_domain_envelope`
- `wrong_domain_no_tag` (byte-identical to the envelope case)
- `wrong_domain_doctor_selfverify`
- `tampered_network_id`
- `tampered_challenge`
- `tampered_ts`
- `tampered_slug`

Regenerate with:

```text
go test ./internal/identity/ -run TestGenerate_SessionStatementVectors -update
```

Do not pass `-update` to `./internal/identity/...`. Other packages use that flag for their own fixtures.

## What the hub must also check

`VerifySessionStatement` returning true is not a login. The hub must also:

- accept the challenge only once, and only while it is unexpired;
- require `network_id` to equal the hub's own network id;
- require `ts` to fall inside the hub's acceptance window;
- resolve `slug` and `key_epoch` to a live, unrevoked registry binding, and verify the signature with that binding's pubkey.

The signer does not do those checks. It only refuses to emit a statement that is structurally invalid or that does not verify under its own binding.
