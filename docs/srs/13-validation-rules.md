# Validation Rules

> See also: [frontmatter schema](04-frontmatter-schema.md), [mutation model](06-mutation-model.md), [response shapes](09-response-shapes.md)

## Overview

Validation runs as part of `frontmatter validate` and is checked before any mutation.

## Rules

| Rule ID | Rule | Severity | Applies to |
|---------|------|----------|------------|
| `missing_id` | `id` missing | error | Domain note detection |
| `missing_type` | `type` missing | error | Domain note detection |
| `partial_domain` | `id` + `type` partially present (one without the other) | warning | All `.md` files |
| `duplicate_id` | Duplicate `id` across vault | error | All domain notes |
| `missing_required_field` | Required field missing for type (per registry) | error | Domain notes |
| `unknown_type` | Unrecognized `type` value (not in registry) | warning | Domain notes |
| `invalid_status` | Invalid `status` for type (not in registry statuses) | warning | Domain notes |
| `scalar_list_field` | Scalar `aliases` or `tags` (not list) | error | All notes with these fields |
| `broken_reference` | Broken stable-ID reference in `*_ids` fields | warning | Domain notes |
| `frontmatter_wikilink` | Frontmatter wikilink used instead of stable ID in `*_ids` | warning | Domain notes |
| `non_snake_case_key` | Non-snake_case frontmatter key | warning | All notes |
| `invalid_date` | Invalid date format in `created`/`updated`/`due` | error | Domain notes |
| `malformed_markers` | Malformed VAULTMIND generated markers (START without END, etc.) | error | All notes |
| `duplicate_markers` | Duplicate VAULTMIND markers for same `section_key` | error | All notes |

## Issue Response Shape

Each issue in the `frontmatter validate` response:

```json
{
  "path": "projects/old-thing.md",
  "id": "proj-old-thing",
  "severity": "error",
  "rule": "missing_required_field",
  "message": "Type 'project' requires field 'status'",
  "field": "status"
}
```

Optional fields in the issue object: `field`, `value` (the offending value), `candidates` (for ambiguous/duplicate cases).

## Pre-mutation Validation

Before any write, the mutation engine validates:

1. Target note exists and resolves unambiguously
2. Target is a domain note (for operations requiring it)
3. The field being modified is allowed by the type schema (or `--allow-extra`)
4. `id` and `type` are not being modified
5. The resulting frontmatter passes all applicable validation rules
6. Generated region markers are well-formed (for dataview operations)
