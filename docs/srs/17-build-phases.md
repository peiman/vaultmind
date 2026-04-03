# Build Phases and Acceptance Criteria

> See also: [CLI reference](11-cli-reference.md), [architecture](02-architecture.md)

## Phase 1 — Core Index and Query

- Scaffold from `ckeletin-go`
- Vault scanner with exclude patterns
- Parser: frontmatter, headings, wikilinks, embeds, tags, code fences, block IDs
- SQLite schema and full rebuild indexing
- Note classification (domain vs unstructured)
- Entity resolution with collision/ambiguity handling
- Commands: `index`, `search`, `resolve`, `note get`, `links out`, `links in`, `frontmatter validate`, `doctor`
- JSON output for all implemented commands
- Type registry and config loading

## Phase 2 — Mutation and Git

- Frontmatter mutation: `set`, `unset`, `merge`, `normalize`
- Dry-run, diff preview for all mutations
- Generated Dataview region rendering and linting
- Git state detection and policy matrix enforcement
- `--commit` flag with structured commit messages
- Incremental indexing (mtime + hash)
- Unresolved link diagnostics
- Plan file parsing and `apply` command

## Phase 3 — Memory and API

- `memory recall` with configurable depth and edge filters
- `memory related` with explicit/inferred/mixed modes
- `memory context-pack` with token budget
- Inferred edges: alias mentions, tag overlap
- `links neighbors` with depth traversal
- Note templates and `note create`
- Optional API server wrapping core library
- Schema packs (pre-built type registries)

---

## Acceptance Criteria

VaultMind v1 is acceptable when:

1. A Git-backed vault with standardized frontmatter can be indexed and queried without Obsidian.
2. Domain notes and unstructured notes are classified and handled correctly.
3. Entity resolution handles collisions and ambiguity explicitly.
4. Outbound and inbound links are returned for any indexed note, with edge type and confidence.
5. Associative memory queries traverse the graph with configurable depth and filters.
6. Context packs respect token budgets and include provenance metadata.
7. Frontmatter mutations produce minimal, reviewable diffs.
8. Generated Dataview regions are managed safely with checksum-based hand-edit detection.
9. Git policy matrix is enforced before all mutations.
10. Plan files execute atomically with rollback on failure.
11. All agent-relevant commands return stable JSON matching the contract in [agent contract](08-agent-contract.md) and [response shapes](09-response-shapes.md).
12. Validation reports broken references, duplicate IDs, schema violations, and malformed markers.
13. The tool runs on macOS and Linux without Obsidian installed.
