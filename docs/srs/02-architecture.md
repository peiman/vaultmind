# Architecture

> See also: [data model](03-data-model.md), [storage](12-storage-model.md), [config](18-config-spec.md)

## Layers

| Layer | Responsibility |
|-------|---------------|
| Vault layer | Plain Markdown, YAML frontmatter, Obsidian-compatible links, filesystem structure |
| Memory layer | Parsing, indexing, graph construction, associative recall, validation, query |
| Change-control layer | Git-aware diffing, safety checks, commit generation, mutation workflows |

## Components

1. **Vault scanner** — discovers notes and assets recursively, respects exclude patterns
2. **Parser** — extracts frontmatter, headings, links, tags, code fences, block structure
3. **Indexer** — writes normalized entities and edges into SQLite
4. **Memory engine** — associative recall, neighborhood traversal, context packaging
5. **Mutation engine** — safe bounded writes with plan-then-apply workflow
6. **Git integration layer** — repo state inspection, diffs, staging, commits
7. **CLI** — user and agent commands
8. **API server** (optional) — wraps the same core library

## Go Package Structure

```
cmd/vaultmind/       # CLI entrypoint
internal/vault/      # Vault scanner, file discovery
internal/parser/     # Markdown/frontmatter/link extraction
internal/schema/     # Type registry, field validation
internal/index/      # SQLite indexer, incremental updates
internal/graph/      # Edge storage, traversal queries
internal/memory/     # Recall, related, context-pack
internal/mutate/     # Frontmatter writes, generated regions, plan execution
internal/dataview/   # Dataview block detection, template rendering, linting
internal/git/        # Repo state, policy enforcement, staging, commits
internal/cli/        # Cobra command definitions
internal/config/     # Config loading, defaults, validation
```

Each package should have a clear single responsibility and communicate through well-defined interfaces. The core library (everything under `internal/`) is shared between CLI and API server.
