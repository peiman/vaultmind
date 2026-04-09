# VaultMind Design Specs & Implementation Plans

Organized by major version. Each directory contains design specs (`*-design.md`) and implementation plans (`*-plan.md`).

## v1/ — Core System (Indexing, Graph, Memory Commands)

The foundation: vault parsing, SQLite indexing, graph traversal, memory commands, FTS search.

| Spec | What |
|------|------|
| p2a — Git Integration | Git status tracking for vault files |
| p2b — Mutation Engine | Frontmatter set/unset/merge/normalize |
| p2c — Incremental Indexing | Mtime + hash change detection |
| p2d — Dataview Regions | Generated sections in notes |
| p2e — Plan Files | Apply command for bulk operations |
| p3a — Inferred Edges | Alias mentions + tag overlap detection |
| p3b — Graph Traversal | BFS neighbors, spreading activation |
| p3c — Memory Commands | recall, related, context-pack |
| p3d — Note Create | Note creation with templates |
| session04-fixes | Error handling, JSON envelopes, doctor |
| v1.1 | Bug fixes, CLI DX, context-pack improvements |
| v1.2 | ask command, recall defaults, max-items/slim |
| v2-scope | Deferred items and v2 feature roadmap |

## v2/ — Embedding Retrieval

Semantic search, hybrid retrieval, BGE-M3 3-in-1.

| Spec | What |
|------|------|
| v2-embedding-design | Dense embedding pipeline: MiniLM, brute-force cosine, N-way RRF |
| v2-embedding-plan | 11-task implementation plan for embedding retrieval |
| bge-m3-upgrade-design | BGE-M3 3-in-1: dense+sparse+ColBERT heads, ORT backend |
| bge-m3-upgrade-plan | 11-task implementation plan for BGE-M3 upgrade |

## v3/ — Experiment Framework & Activation Scoring

Data-driven improvement through instrumentation, shadow scoring, and outcome tracking.

| Spec | What |
|------|------|
| experiment-framework-design | General-purpose experiment instrumentation, telemetry tiers, shadow scoring |
| *(upcoming)* activation-scoring-design | Compressed idle time + dual strength, first experiment |

## Conventions

- **Naming**: `YYYY-MM-DD-<topic>-design.md` for specs, `YYYY-MM-DD-<topic>-plan.md` for plans
- **Design first**: Every feature gets a design spec approved before implementation
- **Plans reference specs**: Implementation plans always link to their design spec
- **Version directories**: Group by major version / feature area
