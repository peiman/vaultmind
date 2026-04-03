# Frontmatter Schema

> See also: [data model](03-data-model.md), [validation rules](13-validation-rules.md), [config spec](18-config-spec.md)

## Metadata Tiers

| Tier | Scope | Fields |
|------|-------|--------|
| Core | Required on all domain notes | `id`, `type`, `created`, `updated` |
| Graph | Required or optional per type | `title`, `status`, `aliases`, `tags`, `parent_id`, `related_ids`, `source_ids` |
| Domain | Type-specific extensions | `owner_id`, `project_id`, `area`, `due`, `url`, `score`, etc. |

## Core Field Specifications

| Field | Required | Type | Rules |
|-------|----------|------|-------|
| `id` | yes | string | Globally unique. Immutable after creation. |
| `type` | yes | string | Controlled vocabulary from [type registry](18-config-spec.md). |
| `created` | yes | string | ISO 8601 date (`YYYY-MM-DD`) or datetime (`YYYY-MM-DDTHH:MM:SS`). Set once. |
| `updated` | yes | string | ISO 8601. Human/Obsidian-managed. VaultMind reads but does not write this field. |
| `vm_updated` | yes | string | ISO 8601. **VaultMind-managed.** Updated on every VaultMind mutation. |

> **Why two timestamp fields?** Obsidian plugins (e.g., `update-time-on-edit`) commonly auto-update the `updated` field on every save. If VaultMind also wrote `updated`, the plugin would immediately rewrite it, causing hash conflicts that refuse subsequent VaultMind writes. `vm_updated` is VaultMind's own timestamp, immune to plugin interference. See [risks](16-risks.md).
| `title` | conditional | string | Required if display title differs from filename. |
| `status` | conditional | string | Required on types with lifecycle. Values from type registry. |
| `aliases` | optional | list of strings | YAML list only. Never scalar. |
| `tags` | optional | list of strings | YAML list only. Never scalar. Never comma-delimited. |
| `parent_id` | optional | string | Stable ID reference to parent note. |
| `related_ids` | optional | list of strings | Stable ID references. YAML list only. |
| `source_ids` | optional | list of strings | Stable ID references. YAML list only. |

## Type Registry

Defined in [config](18-config-spec.md). Per type, the registry specifies:

- Which Graph-tier fields are required vs optional
- Which Domain-tier fields are recognized
- Which `status` values are valid
- Template path for `note create`

Example:

```yaml
types:
  project:
    required: [status, title]
    optional: [owner_id, due, area, tags, aliases, related_ids, source_ids]
    statuses: [active, paused, completed, cancelled]
    template: templates/project.md
  person:
    required: [title]
    optional: [aliases, tags, related_ids, url]
    statuses: []
    template: templates/person.md
  concept:
    required: [title]
    optional: [aliases, tags, related_ids, source_ids]
    statuses: []
    template: templates/concept.md
```

## Example Frontmatter

```yaml
---
id: proj-payment-retries
type: project
status: active
title: Payment Retries
aliases:
  - Retry Engine
  - Billing Retries
created: 2026-04-03
updated: 2026-04-03
tags:
  - billing
  - payments
owner_id: person-rend
related_ids:
  - concept-idempotency
  - source-stripe-retries
---
```

## Anti-patterns (rejected by validation)

- Scalar `aliases` or `tags` (must be YAML lists)
- Comma-delimited tags
- Frontmatter wikilinks as the sole relationship mechanism (use stable IDs)
- Overloading tags to encode typed relations
- Requiring every optional field on every note
- Treating Dataview output as canonical
