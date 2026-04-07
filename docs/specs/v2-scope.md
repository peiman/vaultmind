# VaultMind v2 Scope

**Date:** 2026-04-07
**Source:** Deferred items from v1.1, expert panel findings, dogfooding evaluation

---

## Deferred from v1.1

### Embedding Retrieval Architecture (P1.1e E2-E5 + P1.1f F2)

The Retriever interface is wired in (v1.1 E1). The schema has the embedding column (v1.1 F1). v2 builds on this foundation:

| Item | Description | Prerequisite |
|------|-------------|-------------|
| **E2** | Cross-query BM25 normalization — corpus-wide statistics for query-independent scoring | Decide normalization strategy (sigmoid vs z-score) |
| **E3** | EmbeddingRetriever — semantic vector search via stored embeddings | Choose embedding provider (OpenAI API, local model, etc.) |
| **E4** | Query-independent score normalization — z-score using corpus BM25 stats | E2 |
| **F2** | Embedding computation during indexing — `--embed` flag, configurable model | E3 (need Embedder interface) |
| **E5** | HybridRetriever — weighted combination of FTS + embedding scores | E3, E4 |

**Design decisions needed before implementation:**
- Embedding provider: OpenAI `text-embedding-3-small`, local `all-MiniLM-L6-v2`, or pluggable?
- Vector similarity: cosine in SQLite (pure Go), or external FAISS/Qdrant?
- Index-time vs query-time embedding: compute during `index --embed` or on-demand?
- Cost: embedding API calls per note, budget per vault size tier

### Pre-commit Hook (P1.1c C4)

Add a lefthook/pre-commit hook that runs `vaultmind doctor` on the vault and fails if Obsidian-incompatible links are found. Deferred because the hook depends on having the binary built, which is awkward in CI.

### Test Fixture Migration (P1.1g G1)

Migrate remaining tests from live vault (`../../vaultmind-vault`) to `test/fixtures/testvault/`. Do incrementally when touching test files — not a standalone effort.

---

## New v2 Features (from expert panel and dogfooding)

### Access Frequency Tracking

Schema migration 004 added `last_accessed_at` and `access_count` columns. v2 should:
- Increment `access_count` and set `last_accessed_at` on `note get` and `memory recall`
- Use access frequency in context-pack priority ordering (ACT-R-style base-level activation)
- Add `--sort recency` option to search

### Rebuild/Incremental Error JSON Envelope

`cmd/index.go` routes vault-path and config errors through JSON, but `Rebuild()` and `Incremental()` errors still return raw text with `--json`. Wrap these too.

### Index Command Vault Errors — Remaining Gap

The `index` command's `Rebuild()` / `Incremental()` error paths (lines 53-63 of `cmd/index.go`) bypass the JSON envelope. These should use `classifyVaultError` or a new `classifyIndexError`.

### Embed Wikilink Guard in Lint

`lint fix-links` regex matches `[[Target]]` inside `![[embed]]` syntax. Guard against rewriting embeds — they don't support the `|display` format.

### Doctor Improvements

- Flag wikilinks to CLI commands (backtick-code vs wikilink convention)
- Detect notes with 0 inbound links (orphans) as warnings
- Detect notes with stale `vm_updated` (older than file mtime)

---

## Architecture Principles for v2

From the expert panel and dogfooding:

1. **Retriever is the extension point.** All new retrieval backends implement `query.Retriever`. No direct `SearchFTS` calls outside the FTS retriever.
2. **Goose manages all schema changes.** No manual ALTER TABLE. Every change is a numbered migration.
3. **Obsidian compatibility is non-negotiable.** All wikilinks use `[[filename|Display]]`. Doctor catches violations. Lint fixes them.
4. **Tests use fixtures, not the live vault.** New tests use `test/fixtures/testvault/` or build their own temp DBs.
5. **Dogfood everything.** Use VaultMind CLI for real work. Bugs found this way are higher priority than theoretical issues.
