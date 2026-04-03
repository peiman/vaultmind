# Data Model

> See also: [frontmatter schema](04-frontmatter-schema.md), [storage model](12-storage-model.md), [entity resolution in glossary](glossary.md)

## Canonical Storage

Canonical content consists of:

- Markdown note bodies
- YAML frontmatter
- Filesystem structure inside the vault
- Git history over those files

## Derived Artifacts (non-canonical, always rebuildable)

- SQLite index
- Backlinks
- Inferred relations
- Unresolved-link diagnostics
- Dataview output
- Generated summaries
- Search indexes
- Context packs

## Note Classification

Every `.md` file is classified as one of two types:

### Domain Note

A Markdown file whose frontmatter contains **both** `id` and `type` fields. Fully indexed: participates in graph resolution, frontmatter validation, associative memory queries, and mutation workflows.

### Unstructured Note

Any `.md` file that lacks `id`, `type`, or both. Partially indexed:

- Body text searchable via FTS
- Outbound links extracted and resolved where possible
- Can appear as link targets
- **Cannot** participate in frontmatter validation
- **Cannot** be targets of stable-ID references
- **Excluded** from associative memory queries requiring typed graph membership

A file with `id` but not `type` (or vice versa) is classified as unstructured and produces a validation warning.

**SQLite key for unstructured notes:** Since unstructured notes lack a stable `id`, they use a synthetic key: `_path:<vault-relative-path>`. This key is unstable across renames but sufficient for FTS and link resolution. The `is_domain` column distinguishes them.

## Identity Model

Every domain note must have an immutable `id` field, unique across the entire vault. Assigned at creation, never changed.

Path and filename are operational properties, not identity. Renames must not break resolution.

**Recommended format:** `{type_prefix}-{slug}` (e.g., `proj-payment-retries`). Not enforced.

## Entity Resolution

VaultMind resolves a reference to a note in this priority order:

| Priority | Tier | Match type |
|----------|------|-----------|
| 1 | `id` | Exact `id` match |
| 2 | `title` | Exact `title` match |
| 3 | `alias` | Exact alias match |
| 4 | `normalized` | Case-insensitive, whitespace-normalized title/alias |
| 5 | `unresolved` | No match — recorded as diagnostic |

### Collision Policy

If multiple notes match at the same tier, VaultMind returns **all matches** with `ambiguous: true` and the candidate list. It never silently picks one.

Agents and CLI commands receiving an ambiguous result must treat it as an error requiring human disambiguation, unless the command explicitly supports multi-match (e.g., `search`).

### Path Shortcut

If the input contains `/` or ends in `.md`, path lookup is tried first before running the resolution tiers.
