# Response Shapes

> JSON response shapes for all commands. See also: [agent contract](08-agent-contract.md), [CLI reference](11-cli-reference.md)
>
> All shapes shown are the `result` field inside the [JSON envelope](08-agent-contract.md).

## `resolve`

```json
{
  "resolved": true,
  "ambiguous": false,
  "input": "Retry Engine",
  "resolution_tier": "alias",
  "matches": [
    {
      "id": "proj-payment-retries",
      "type": "project",
      "title": "Payment Retries",
      "path": "projects/payment-retries.md",
      "status": "active"
    }
  ]
}
```

When ambiguous: `"ambiguous": true`, multiple entries in `matches`.
When unresolved: `"resolved": false`, `"resolution_tier": null`, `"matches": []`.

## `note get`

```json
{
  "id": "proj-payment-retries",
  "type": "project",
  "path": "projects/payment-retries.md",
  "title": "Payment Retries",
  "frontmatter": {
    "status": "active",
    "aliases": ["Retry Engine"],
    "tags": ["billing"],
    "owner_id": "person-rend",
    "related_ids": ["concept-idempotency"],
    "created": "2026-04-03",
    "updated": "2026-04-03"
  },
  "body": "Full markdown body text...",
  "headings": [
    { "level": 2, "title": "Overview", "slug": "overview" }
  ],
  "blocks": [
    { "block_id": "key-metric", "heading": "Overview", "line": 12 }
  ],
  "is_domain_note": true
}
```

## `links out`

```json
{
  "source_id": "proj-payment-retries",
  "links": [
    {
      "target_id": "concept-idempotency",
      "target_title": "Idempotency",
      "target_path": "concepts/idempotency.md",
      "edge_type": "explicit_relation",
      "confidence": "high",
      "origin": "frontmatter.related_ids",
      "resolved": true
    },
    {
      "target_id": null,
      "target_raw": "[[Stripe Webhooks]]",
      "edge_type": "explicit_link",
      "confidence": "high",
      "origin": "body:line:24",
      "resolved": false
    }
  ]
}
```

## `links in`

Same shape as `links out`, with `source_id`/`source_title`/`source_path` instead of `target_*`.

## `search`

```json
{
  "query": "payment retry logic",
  "offset": 0,
  "limit": 20,
  "hits": [
    {
      "id": "proj-payment-retries",
      "type": "project",
      "title": "Payment Retries",
      "path": "projects/payment-retries.md",
      "snippet": "...the retry logic handles transient failures...",
      "score": 0.92,
      "is_domain_note": true
    }
  ],
  "total": 1
}
```

## `memory recall`

```json
{
  "target_id": "proj-payment-retries",
  "depth": 2,
  "max_nodes": 200,
  "max_nodes_reached": false,
  "nodes": [
    {
      "id": "proj-payment-retries",
      "type": "project",
      "title": "Payment Retries",
      "distance": 0,
      "frontmatter": { "...": "..." }
    },
    {
      "id": "concept-idempotency",
      "type": "concept",
      "title": "Idempotency",
      "distance": 1,
      "edge_from_parent": {
        "edge_type": "explicit_relation",
        "confidence": "high"
      },
      "frontmatter": { "...": "..." }
    }
  ],
  "edges": [
    {
      "source_id": "proj-payment-retries",
      "target_id": "concept-idempotency",
      "edge_type": "explicit_relation",
      "confidence": "high"
    }
  ]
}
```

## `memory related`

```json
{
  "target_id": "proj-payment-retries",
  "mode": "mixed",
  "related": [
    {
      "id": "concept-idempotency",
      "type": "concept",
      "title": "Idempotency",
      "edge_type": "explicit_relation",
      "confidence": "high",
      "origin": "frontmatter.related_ids"
    },
    {
      "id": "proj-email-retries",
      "type": "project",
      "title": "Email Retries",
      "edge_type": "tag_overlap",
      "confidence": "low",
      "score": 2.4,
      "shared_tags": ["retries", "billing"]
    }
  ]
}
```

Modes: `explicit` (high-confidence edges only), `inferred` (medium/low only), `mixed` (all).

## `memory context-pack`

```json
{
  "target_id": "proj-payment-retries",
  "budget_tokens": 4096,
  "used_tokens": 3842,
  "budget_exhausted": false,
  "truncated": false,
  "target": {
    "id": "proj-payment-retries",
    "frontmatter": { "...": "..." },
    "body": "Full body text..."
  },
  "context": [
    {
      "id": "concept-idempotency",
      "edge_type": "explicit_relation",
      "confidence": "high",
      "frontmatter": { "...": "..." },
      "body_included": false
    }
  ]
}
```

## `frontmatter validate`

```json
{
  "files_checked": 142,
  "valid": 138,
  "issues": [
    {
      "path": "projects/old-thing.md",
      "id": "proj-old-thing",
      "severity": "error",
      "rule": "missing_required_field",
      "message": "Type 'project' requires field 'status'",
      "field": "status"
    }
  ]
}
```

**`--live` flag:** Validates raw `.md` files on disk instead of the indexed
database. Use when you want to catch schema drift introduced by external
edits *before* the next `vaultmind index` run — for example, before
committing a batch of hand-edited notes. Live mode evaluates the same
rules as the default mode (`unknown_type`, `missing_required_field`,
`invalid_status`), plus `invalid_frontmatter` for unparseable YAML, and
skips `broken_reference` (which requires the indexed link graph).

## `frontmatter set` (with `--dry-run --diff`)

```json
{
  "path": "projects/payment-retries.md",
  "id": "proj-payment-retries",
  "operation": "set",
  "key": "status",
  "old_value": "active",
  "new_value": "paused",
  "dry_run": true,
  "diff": "--- a/...\n+++ b/...\n@@ ...\n-status: active\n+status: paused",
  "git": {
    "repo_detected": true,
    "working_tree_clean": true,
    "target_file_clean": true
  }
}
```

## `note mget`

```json
{
  "notes": [
    {
      "id": "proj-payment-retries",
      "type": "project",
      "path": "projects/payment-retries.md",
      "title": "Payment Retries",
      "frontmatter": { "...": "..." },
      "is_domain_note": true
    }
  ],
  "not_found": ["nonexistent-id"],
  "total": 1
}
```

Default is frontmatter-only. With `--include-body`, each note object also includes `body`, `headings`, `blocks`.

## `apply`

```json
{
  "plan_description": "Pause all billing projects",
  "operations_total": 4,
  "operations_completed": 4,
  "operations": [
    {
      "op": "frontmatter_set",
      "target": "proj-payment-retries",
      "status": "ok",
      "write_hash": "sha256:abc123..."
    },
    {
      "op": "note_create",
      "path": "decisions/pause-billing.md",
      "id": "decision-pause-billing",
      "status": "ok",
      "write_hash": "sha256:def456..."
    }
  ],
  "committed": true,
  "commit_sha": "a1b2c3d"
}
```

On failure, `operations_completed < operations_total`. Failed operation has `"status": "error"` with error object. Subsequent operations show `"status": "skipped"`.

## `vault status`

```json
{
  "vault_path": "/path/to/vault",
  "total_files": 423,
  "domain_notes": 312,
  "unstructured_notes": 111,
  "index_status": "current",
  "index_stale": false,
  "git": {
    "repo_detected": true,
    "branch": "main",
    "working_tree_clean": true
  },
  "types": {
    "project": { "count": 45, "required": ["status", "title"], "statuses": ["active", "paused", "completed", "cancelled"] },
    "concept": { "count": 120, "required": ["title"], "statuses": [] }
  },
  "issues_summary": {
    "errors": 1,
    "warnings": 5
  }
}
```

Cold-start command combining `doctor`, `schema list-types`, `git status`, and index freshness into one call.

## `git status`

```json
{
  "repo_detected": true,
  "branch": "main",
  "detached": false,
  "merge_in_progress": false,
  "rebase_in_progress": false,
  "working_tree_clean": false,
  "staged_files": [],
  "unstaged_files": ["projects/payment-retries.md"],
  "untracked_files": ["scratch/temp.md"]
}
```

## `doctor`

```json
{
  "vault_path": "/path/to/vault",
  "total_files": 423,
  "domain_notes": 312,
  "unstructured_notes": 111,
  "index_status": "current",
  "git_status": "clean",
  "issues": {
    "duplicate_ids": 0,
    "broken_references": 3,
    "missing_required_fields": 1,
    "malformed_markers": 0,
    "unresolved_links": 12,
    "notes_missing_id_or_type": 5
  }
}
```
