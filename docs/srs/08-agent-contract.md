# Agent Contract

> See also: [response shapes](09-response-shapes.md), [CLI reference](11-cli-reference.md)

## Design Requirements

All agent-facing outputs must be:

- **Deterministic** given the same vault state
- **Machine-readable** (JSON)
- **Explicit** about confidence and ambiguity
- **Explicit** about canonical vs derived status
- **Compact** enough for downstream context assembly
- **Safe** for programmatic consumption — structured errors, never bare strings

## JSON Envelope

Every `--json` command returns this envelope:

```json
{
  "command": "string",
  "status": "ok | error | warning",
  "warnings": [
    {
      "code": "string",
      "message": "string",
      "field": "string (optional)"
    }
  ],
  "errors": [
    {
      "code": "string",
      "message": "string",
      "field": "string (optional)",
      "candidates": ["string (optional)"]
    }
  ],
  "result": { },
  "meta": {
    "vault_path": "string",
    "index_hash": "string",
    "timestamp": "ISO 8601"
  }
}
```

- `result` is command-specific — see [response shapes](09-response-shapes.md)
- When `status` is `error`, `result` may be `null`
- `warnings` and `errors` are always arrays (empty if none)
- Each error object has `code` (stable identifier) and `message` (human-readable). Optional fields: `field` (which field caused the error), `candidates` (for ambiguous resolution)
- When `status` is `error` due to ambiguous resolution, `result` is still populated with `ambiguous: true` and the `matches` list, enabling agents to resolve without a second round-trip
- `warnings` use the same structured format as errors: `code` (stable identifier), `message`, optional `field`
- `meta.index_hash` is the SHA-256 of the SQLite database file
- `meta.index_stale` (boolean): `true` if a mutation occurred since the last index rebuild. Agents should re-index or treat graph queries as potentially stale

### Mutation Response Extensions

All mutation responses (`frontmatter set/unset/merge`, `note create`, `dataview render`, `apply`) include:

- `write_hash`: SHA-256 of the file after write. Agents can compare this against a subsequent `note get` to verify the mutation landed.
- `reindex_required`: `true` — signals that graph queries will return stale results until the next `vaultmind index` run.

After any write, VaultMind performs an **implicit incremental re-index** of the affected file(s) before returning. This ensures that the next read command reflects the mutation. If implicit re-indexing is disabled via config, `reindex_required: true` is set instead.

## Human-readable Output

When `--json` is not passed, commands produce human-readable output to stdout. This format is **not stable** and must not be parsed programmatically. Agents should always use `--json`.

Human-readable output should be concise and scannable — tables for list data, key-value pairs for single entities, unified diffs for mutations.

## Stable Output Policy

JSON field names are part of the agent contract:

- Fields may be **added** but never **removed** or **renamed** within a major version
- New optional fields must not change the semantics of existing fields
- Error codes in refusal responses are stable identifiers

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (`status: "ok"` or `"warning"`) |
| 1 | Error (`status: "error"`) |
| 2 | Usage error (invalid flags, missing arguments) |
