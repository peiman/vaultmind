# Session 04 Expert Panel Fixes — Design Spec

**Date:** 2026-04-06
**Source:** [Session 04 Summary](../../reviews/session-04/summary.md)
**Scope:** 8 fixes (5 agent deployment, 3 v2 prerequisites)

---

## Overview

The Session 04 expert panel identified 5 agent-deployment issues (4 blocking + 1 cache invalidation gap) and 3 architectural prerequisites for v2. This spec covers all 8 fixes in two groups:

- **Group A (Items 1–5):** Agent deployment fixes — correctness and reliability bugs
- **Group B (Items 6–8):** v2 prerequisites — architectural improvements

All fixes include TDD: failing tests first, then implementation.

---

## Group A: Agent Deployment Fixes

### Item 1 — BFS Inbound Edge SourceID Inversion

**Problem:** In `internal/graph/traverse.go:173-175`, the inbound edge loop sets `nb.sourceID = nodeID` (the BFS parent) instead of the actual `src_note_id` from the database. For an inbound edge A→B where B is being expanded, the output reports `SourceID=B` when it should report `SourceID=A`. Edge direction is inverted for every node discovered through an inbound edge.

**Impact:** `memory recall` RecallEdge and `links neighbors` TraverseEdge report wrong source for inbound-discovered nodes. Any agent reasoning about link direction receives inverted information.

**Fix:**

File: `internal/graph/traverse.go`

Change the inbound edge loop (line ~173-178) from:
```go
for inRows.Next() {
    var nb neighbor
    nb.sourceID = nodeID
    if err := inRows.Scan(&nb.id, &nb.edgeType, &nb.confidence, &nb.weight); err != nil {
```

To:
```go
for inRows.Next() {
    var nb neighbor
    if err := inRows.Scan(&nb.id, &nb.edgeType, &nb.confidence, &nb.weight); err != nil {
        ...
    }
    nb.sourceID = nb.id  // actual src_note_id, not BFS parent
```

For inbound edges, `nb.id` is scanned from `src_note_id` (the actual edge source). Setting `nb.sourceID = nb.id` after scan makes `TraverseEdge.SourceID` report the correct edge direction.

**Test:** Add test in `internal/graph/traverse_test.go`:
- Create edge A→B in the links table
- Traverse from B with depth 1
- Assert node A is discovered
- Assert `EdgeFrom.SourceID == A` (not B)
- Also verify outbound edges still report `SourceID == nodeID` (unchanged behavior)

**Files changed:**
- `internal/graph/traverse.go` — 2 lines moved
- `internal/graph/traverse_test.go` — new test case

---

### Item 2 — OpenVaultDB Errors Bypass JSON Envelope

**Problem:** All 24 commands supporting `--json` share this pattern:
```go
vdb, err := cmdutil.OpenVaultDB(vaultPath)
if err != nil {
    return err  // raw text, even with --json
}
```

Three error classes bypass the JSON envelope: vault path not found, config load failure, database open failure. An agent parsing JSON will crash.

**Fix:**

Add a new helper to `internal/cmdutil/helpers.go`:

```go
// OpenVaultDBOrWriteErr opens the vault DB. On failure, if --json is set,
// it writes a JSON error envelope and returns ErrAlreadyWritten so the
// caller can return nil (the error was already written to stdout).
func OpenVaultDBOrWriteErr(cmd *cobra.Command, vaultPath, commandName string) (*VaultDB, error) {
    vdb, err := OpenVaultDB(vaultPath)
    if err != nil {
        if isJSONOutput(cmd) {
            _ = WriteJSONError(cmd.OutOrStdout(), commandName, "vault_error", err.Error())
            return nil, ErrAlreadyWritten
        }
        return nil, err
    }
    return vdb, nil
}
```

Where `ErrAlreadyWritten` is a sentinel error and `isJSONOutput` checks the `--json` flag:

```go
var ErrAlreadyWritten = fmt.Errorf("error already written to output")

func isJSONOutput(cmd *cobra.Command) bool {
    jsonFlag, _ := cmd.Flags().GetBool("json")
    return jsonFlag
}
```

Each command's `RunE` handles the sentinel:
```go
vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "search")
if err != nil {
    if errors.Is(err, cmdutil.ErrAlreadyWritten) {
        return nil  // JSON error already written
    }
    return err
}
```

**Affected commands (24):**
- `cmd/apply.go`
- `cmd/dataview_lint.go`
- `cmd/dataview_render.go`
- `cmd/doctor.go`
- `cmd/frontmatter_helpers.go` (shared by set/unset/merge/normalize)
- `cmd/frontmatter_validate.go`
- `cmd/git_status.go`
- `cmd/index.go`
- `cmd/links_in.go`
- `cmd/links_neighbors.go`
- `cmd/links_out.go`
- `cmd/memory_context_pack.go`
- `cmd/memory_recall.go`
- `cmd/memory_related.go`
- `cmd/note_create.go`
- `cmd/note_get.go`
- `cmd/note_mget.go`
- `cmd/resolve.go`
- `cmd/schema_list_types.go`
- `cmd/search.go`
- `cmd/vault_status.go`

**Test:** Add test in `internal/cmdutil/helpers_test.go`:
- Call `OpenVaultDBOrWriteErr` with a non-existent vault path and a mock `--json` flag
- Assert JSON error envelope is written to output with `code: "vault_error"`
- Assert returned error is `ErrAlreadyWritten`
- Also test without `--json`: assert raw error is returned

**Files changed:**
- `internal/cmdutil/helpers.go` — add `OpenVaultDBOrWriteErr`, `ErrAlreadyWritten`, `isJSONOutput`
- `internal/cmdutil/helpers_test.go` — new test
- 21 `cmd/*.go` files — replace `OpenVaultDB` calls (frontmatter_helpers covers 4 commands)

---

### Item 3 — SearchResult.Total Returns Page Count

**Problem:** `Total: len(results)` returns the number of results on the current page (at most `limit`), not the total number of matching documents across the entire index.

**Fix:**

Add `CountFTS` to `internal/index/fts.go`:

```go
func CountFTS(d *DB, query string, filters ...SearchFilters) (int, error) {
    query = strings.TrimSpace(query)
    if query == "" {
        return 0, nil
    }
    query = sanitizeFTSQuery(query)

    var f SearchFilters
    if len(filters) > 0 {
        f = filters[0]
    }

    q := `SELECT COUNT(*) FROM fts_notes f
          JOIN notes n ON n.id = f.note_id
          WHERE fts_notes MATCH ?`
    args := []interface{}{query}

    if f.Type != "" {
        q += " AND n.type = ?"
        args = append(args, f.Type)
    }
    if f.Tag != "" {
        q += " AND n.id IN (SELECT note_id FROM tags WHERE tag = ?)"
        args = append(args, f.Tag)
    }

    var count int
    if err := d.QueryRow(q, args...).Scan(&count); err != nil {
        return 0, fmt.Errorf("FTS count: %w", err)
    }
    return count, nil
}
```

Update `internal/query/search.go` `RunSearch` to call `CountFTS`:

```go
total, err := index.CountFTS(db, cfg.Query, filters)
if err != nil {
    return fmt.Errorf("counting: %w", err)
}
// ... existing search logic ...
Total: total,  // was: len(results)
```

**Test:** Add test in `internal/index/fts_test.go`:
- Index 10 notes with varying content
- Search with `limit=3, offset=0`
- Assert `len(results) == 3` (page count)
- Assert `CountFTS` returns 10 (or whatever the actual total is)
- Test with type/tag filters: verify count matches filter

**Files changed:**
- `internal/index/fts.go` — add `CountFTS`
- `internal/index/fts_test.go` — new test
- `internal/query/search.go` — call `CountFTS`, set `Total` correctly

---

### Item 4 — Missing COLLATE NOCASE Indexes

**Problem:** No index on `notes(title)` or `aliases(alias)`. `ResolveLinks` does case-insensitive lookups on these columns — full table scans at every link resolution pass.

**Fix:** Add two CREATE INDEX statements. With the goose migration approach (Item 8), these go into migration 002. Without goose, they go into `applySchema()`. Since Item 8 introduces goose, these indexes will be in the second migration file.

```sql
-- +goose Up
CREATE INDEX IF NOT EXISTS idx_notes_title ON notes(title COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_aliases_alias ON aliases(alias COLLATE NOCASE);
```

**Test:** After running migrations, verify indexes exist:
```sql
SELECT name FROM sqlite_master WHERE type='index' AND name IN ('idx_notes_title', 'idx_aliases_alias')
```

**Files changed:**
- `internal/index/migrations/002_add_title_alias_indexes.sql` — new migration file

---

### Item 5 — Populate meta.index_hash

**Problem:** `IndexHash` field is defined in envelope Meta and the computation function exists in `cmdutil/helpers.go`, but no command ever populates it. The field ships as empty string.

**Fix:**

1. Compute the hash once when opening the vault DB. Add a field to `VaultDB`:

```go
type VaultDB struct {
    DB        *index.DB
    Config    *vault.Config
    Reg       *schema.Registry
    dbPath    string
    indexHash string  // computed once on open
}
```

In `OpenVaultDB`, after opening the DB, compute the hash:
```go
vdb := &VaultDB{DB: db, Config: cfg, Reg: reg, dbPath: dbPath}
vdb.indexHash = vdb.IndexHash()
return vdb, nil
```

2. Modify `WriteJSON` to accept the hash and set it:

```go
func WriteJSON(w io.Writer, command string, result interface{}, vaultPath, indexHash string) error {
    env := envelope.OK(command, result)
    env.Meta.VaultPath = vaultPath
    env.Meta.IndexHash = indexHash
    return json.NewEncoder(w).Encode(env)
}
```

3. All callers of `WriteJSON` pass `vdb.GetIndexHash()` (a simple getter).

4. Commands that build envelopes manually set `env.Meta.IndexHash = vdb.GetIndexHash()`.

**Getter method:**
```go
func (v *VaultDB) GetIndexHash() string {
    return v.indexHash
}
```

**Test:** In `internal/cmdutil/helpers_test.go`:
- Open a vault DB with a known index
- Assert `GetIndexHash()` returns a non-empty 64-char hex string
- Assert the hash is consistent across multiple calls (cached, not recomputed)
- Assert the hash changes after modifying the DB and re-opening

**Files changed:**
- `internal/cmdutil/helpers.go` — add `indexHash` field, `GetIndexHash`, update `WriteJSON` signature
- All `cmd/*.go` files that call `WriteJSON` — pass `indexHash` argument
- `internal/cmdutil/helpers_test.go` — new test

---

## Group B: v2 Prerequisites

### Item 6 — BM25 Score Normalization to 0–1

**Problem:** Search scores are negated BM25 floats (positive = better) but not in the 0–1 range. Cannot be blended with graph proximity or recency.

**Fix:**

After collecting all results in `SearchFTS`, apply min-max normalization:

```go
if len(results) > 0 {
    minScore := results[0].Score
    maxScore := results[0].Score
    for _, r := range results[1:] {
        if r.Score < minScore { minScore = r.Score }
        if r.Score > maxScore { maxScore = r.Score }
    }
    spread := maxScore - minScore
    if spread > 0 {
        for i := range results {
            results[i].Score = (results[i].Score - minScore) / spread
        }
    } else {
        // All scores equal — set all to 1.0
        for i := range results {
            results[i].Score = 1.0
        }
    }
}
```

This runs after the negation step (line ~88-90) and before returning.

**Test:** In `internal/index/fts_test.go`:
- Index notes with varying relevance to a query
- Search and assert all scores are in [0.0, 1.0]
- Assert the most relevant result has score 1.0
- Assert the least relevant result has score 0.0 (when spread > 0)
- Single-result case: score is 1.0

**Files changed:**
- `internal/index/fts.go` — ~15 lines after the result loop
- `internal/index/fts_test.go` — new test case

---

### Item 7 — Retriever Interface

**Problem:** `SearchFTS` is called directly with no abstraction. Adding a second retrieval backend requires forking the call site.

**Fix:**

Create `internal/query/retriever.go`:

```go
package query

import (
    "context"

    "github.com/peiman/vaultmind/internal/index"
)

// ScoredResult is a retrieval hit with a normalized score.
type ScoredResult struct {
    ID       string  `json:"id"`
    Type     string  `json:"type"`
    Title    string  `json:"title"`
    Path     string  `json:"path"`
    Snippet  string  `json:"snippet"`
    Score    float64 `json:"score"`
    IsDomain bool    `json:"is_domain_note"`
}

// Retriever abstracts a retrieval backend (FTS, embedding, hybrid).
type Retriever interface {
    // Search returns scored results and the total matching count.
    // Scores are normalized to the [0, 1] range.
    Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error)
}
```

Create `internal/query/fts_retriever.go`:

```go
package query

import (
    "context"

    "github.com/peiman/vaultmind/internal/index"
)

// FTSRetriever wraps the SQLite FTS5 search as a Retriever.
type FTSRetriever struct {
    DB *index.DB
}

func (r *FTSRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
    results, err := index.SearchFTS(r.DB, query, limit, offset, filters)
    if err != nil {
        return nil, 0, err
    }

    total, err := index.CountFTS(r.DB, query, filters)
    if err != nil {
        return nil, 0, err
    }

    scored := make([]ScoredResult, len(results))
    for i, r := range results {
        scored[i] = ScoredResult{
            ID: r.ID, Type: r.Type, Title: r.Title,
            Path: r.Path, Snippet: r.Snippet,
            Score: r.Score, IsDomain: r.IsDomain,
        }
    }
    return scored, total, nil
}
```

Update `RunSearch` to accept a `Retriever` instead of calling `SearchFTS` directly. The `cmd/search.go` constructs an `FTSRetriever{DB: vdb.DB}` and passes it. `SearchResult.Hits` changes type from `[]index.FTSResult` to `[]ScoredResult` — the fields are identical, but the type moves from `index` to `query` package ownership.

**Test:** In `internal/query/fts_retriever_test.go`:
- Create an `FTSRetriever` with a real SQLite DB
- Call `Search` and assert results and total are correct
- Verify the `Retriever` interface is satisfied at compile time: `var _ Retriever = (*FTSRetriever)(nil)`

**Files changed:**
- `internal/query/retriever.go` — new file (interface + ScoredResult type)
- `internal/query/fts_retriever.go` — new file (FTSRetriever implementation)
- `internal/query/fts_retriever_test.go` — new test file
- `internal/query/search.go` — update `RunSearch` to accept `Retriever`
- `cmd/search.go` — construct `FTSRetriever` and pass to `RunSearch`

---

### Item 8 — Schema Migration with goose v3

**Problem:** No migration mechanism. `applySchema()` uses a monolithic DDL block with `CREATE TABLE IF NOT EXISTS`. Adding columns (e.g., embedding BLOB) requires manual ALTER TABLE with no upgrade path.

**Fix:**

1. Add dependency: `go get github.com/pressly/goose/v3`
2. Run `task check:license:source` to verify transitive deps (go-retry: MIT, multierr: MIT)

3. Create migration directory: `internal/index/migrations/`

4. Migration 001 — baseline schema (`001_baseline_schema.sql`):
```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS notes ( ... );
-- full current schema, identical to applySchema() DDL
CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(type);
-- ... all tables, indexes, FTS virtual table ...
```

All statements use `IF NOT EXISTS` so this is safe for databases that already have the schema.

5. Migration 002 — title/alias indexes (`002_add_title_alias_indexes.sql`):
```sql
-- +goose Up
CREATE INDEX IF NOT EXISTS idx_notes_title ON notes(title COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_aliases_alias ON aliases(alias COLLATE NOCASE);
```

6. Update `internal/index/db.go`:

```go
import (
    "embed"
    "github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

func (d *DB) applyMigrations() error {
    provider, err := goose.NewProvider(
        goose.DialectSQLite3,
        d.db,
        migrations,
    )
    if err != nil {
        return fmt.Errorf("creating migration provider: %w", err)
    }
    if _, err := provider.Up(context.Background()); err != nil {
        return fmt.Errorf("running migrations: %w", err)
    }
    return nil
}
```

7. Replace `applySchema()` call in `Open()` with `applyMigrations()`.

8. Keep pragmas (WAL, foreign_keys) separate — they run before migrations.

**Backward compatibility:** Existing databases have no `goose_db_version` table. Goose will create it and run migration 001. Since all statements use `IF NOT EXISTS`, the migration is a no-op on the schema but registers the version. Migration 002 then adds the new indexes.

**Test:** In `internal/index/db_test.go`:
- Open a fresh DB — assert all tables and indexes exist
- Open an existing DB (created with old applySchema) — assert migration 002 adds the new indexes
- Assert `goose_db_version` table exists with version 2

**Files changed:**
- `go.mod`, `go.sum` — add goose v3
- `internal/index/migrations/001_baseline_schema.sql` — new file (current full schema)
- `internal/index/migrations/002_add_title_alias_indexes.sql` — new file
- `internal/index/db.go` — replace `applySchema()` with `applyMigrations()`, keep pragmas
- `internal/index/db_test.go` — migration tests

---

## Dependency Between Items

```
Item 8 (goose) ← Item 4 (indexes go into migration 002)
Item 3 (CountFTS) ← Item 7 (Retriever wraps CountFTS)
Item 2 (OpenVaultDBOrWriteErr) + Item 5 (indexHash) both modify cmdutil/helpers.go
```

**Recommended implementation order:**
1. Item 1 (BFS edge fix — isolated, trivial)
2. Item 8 (goose migration — foundation for Item 4)
3. Item 4 (indexes — migration 002)
4. Item 6 (BM25 normalization — isolated)
5. Item 3 (CountFTS — needed by Item 7)
6. Item 7 (Retriever interface — depends on Item 3)
7. Item 5 (index_hash — modifies helpers.go)
8. Item 2 (OpenVaultDB envelope — modifies helpers.go + all commands, do last)

---

## Acceptance Criteria

- [ ] `task check` passes (all 23 quality checks)
- [ ] Coverage stays >= 85.5% (current baseline)
- [ ] All 8 items have failing tests before implementation
- [ ] `memory recall` returns correct `source_id` for inbound-discovered edges
- [ ] `vaultmind search --json --vault /nonexistent` returns JSON error envelope, not raw text
- [ ] `SearchResult.Total` returns total matching docs, not page count
- [ ] `EXPLAIN QUERY PLAN` for title/alias lookups shows index usage
- [ ] `meta.index_hash` is non-empty in all JSON envelope responses
- [ ] BM25 scores are in [0.0, 1.0] range
- [ ] `var _ Retriever = (*FTSRetriever)(nil)` compiles
- [ ] `goose_db_version` table exists after DB open, version >= 2
- [ ] Existing databases (no goose table) migrate cleanly on open
