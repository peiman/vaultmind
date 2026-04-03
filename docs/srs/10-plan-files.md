# Plan Files

> See also: [mutation model](06-mutation-model.md), [CLI reference](11-cli-reference.md), [git model](07-git-model.md)

## Overview

The `apply` command accepts a plan file: a JSON document describing a batch of mutations to execute atomically.

## Format

```json
{
  "version": 1,
  "description": "Pause all billing projects",
  "operations": [
    {
      "op": "frontmatter_set",
      "target": "proj-payment-retries",
      "key": "status",
      "value": "paused"
    },
    {
      "op": "frontmatter_set",
      "target": "proj-billing-dashboard",
      "key": "status",
      "value": "paused"
    },
    {
      "op": "generated_region_render",
      "target": "proj-payment-retries",
      "section_key": "related",
      "template": "related"
    },
    {
      "op": "note_create",
      "path": "decisions/pause-billing.md",
      "type": "decision",
      "frontmatter": {
        "title": "Pause billing projects",
        "status": "accepted",
        "related_ids": ["proj-payment-retries", "proj-billing-dashboard"]
      },
      "body": "## Context\n\nBilling projects paused pending Q3 review.\n"
    }
  ]
}
```

## Supported Operations

| `op` | Required fields | Notes |
|------|----------------|-------|
| `frontmatter_set` | `target`, `key`, `value` | |
| `frontmatter_unset` | `target`, `key` | |
| `frontmatter_merge` | `target`, `fields` (object) | Merges key-value pairs into frontmatter |
| `generated_region_render` | `target`, `section_key`, `template` | |
| `note_create` | `path`, `type`, `frontmatter` | `body` is optional (defaults to template body or empty) |

## Execution Semantics

1. **Parse and validate** entire plan structure before executing any operation
2. **Check Git policy** for the vault state
3. **Execute** operations in order. For each operation:
   - **Resolve** the `target` reference at execution time (not upfront). This allows `note_create` in an earlier operation to be referenced by `frontmatter_set` in a later operation within the same plan.
   - Apply the operation. Abort on first failure.
4. **Rollback** on abort: best-effort restoration. All previously written files are restored from pre-operation copies. Note: if the process is killed mid-plan, partial writes may persist — a subsequent `vaultmind index` will reflect the actual vault state.
5. **Commit** on success with `--commit`: single commit covering all changes, message derived from plan `description`

## Plan Validation Errors

| Condition | Error |
|-----------|-------|
| Unknown `op` value | `unknown_operation` |
| Missing required field for op | `missing_field` |
| `target` does not resolve | `unresolved_target` |
| `target` resolves ambiguously | `ambiguous_target` |
| `note_create` path already exists | `path_exists` |
| `note_create` type not in registry | `unknown_type` |
| `version` != 1 | `unsupported_version` |
