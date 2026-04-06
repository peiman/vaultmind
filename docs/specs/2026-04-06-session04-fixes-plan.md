# Session 04 Expert Panel Fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 5 agent-deployment bugs and add 3 v2 architectural prerequisites identified by the Session 04 expert panel.

**Architecture:** 8 targeted fixes across graph traversal, CLI error handling, FTS search, schema indexing, envelope metadata, score normalization, retrieval abstraction, and schema migration. Each fix is isolated with TDD and atomic commits.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), goose v3 (pressly/goose), testify, cobra

**Spec:** [Session 04 Fixes Design](2026-04-06-session04-fixes-design.md)

**IMPORTANT:** Always use `task` commands (`task test`, `task lint`, `task check`) instead of raw `go test`, `golangci-lint`, etc. The ONLY exception is `go test -v -run TestName ./path/...` for debugging a specific test. See CLAUDE.md for full rules.

---

### Task 1: Fix BFS inbound edge sourceID inversion

**Files:**
- Modify: `internal/graph/traverse.go:173-178`
- Modify: `internal/graph/traverse_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/graph/traverse_test.go`:

```go
func TestTraverse_InboundEdgeSourceID(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	// Find a note that has an inbound link (some other note links TO it).
	// "concept-spreading-activation" is linked from "proj-vaultmind" via body wikilinks.
	// Traverse from "concept-spreading-activation" depth 1 — should discover
	// the source note with EdgeFrom.SourceID set to the ACTUAL edge source,
	// not the BFS parent.
	result, err := r.Traverse(graph.TraverseConfig{
		StartID:       "concept-spreading-activation",
		MaxDepth:      1,
		MinConfidence: "low",
		MaxNodes:      200,
	})
	require.NoError(t, err)
	require.Greater(t, len(result.Nodes), 1, "should discover at least one neighbor")

	// For every discovered node, if EdgeFrom exists, SourceID must NOT equal
	// the start node's ID when the edge is inbound (the actual source is the
	// neighbor, not the traversal parent).
	for _, n := range result.Nodes[1:] {
		require.NotNil(t, n.EdgeFrom)
		// The key assertion: SourceID should be the actual edge source from the
		// links table, which for inbound edges is the discovered node itself
		// (src_note_id), not "concept-spreading-activation" (the BFS parent).
		assert.NotEqual(t, result.StartID, n.EdgeFrom.SourceID,
			"inbound edge for node %s should have SourceID != start node; got SourceID=%s",
			n.ID, n.EdgeFrom.SourceID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestTraverse_InboundEdgeSourceID ./internal/graph/...`
Expected: FAIL — SourceID equals the start node for inbound-discovered nodes

- [ ] **Step 3: Fix the inbound edge sourceID assignment**

Edit `internal/graph/traverse.go`. In the `queryNeighbors` function, change the inbound edge loop from:

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
			return nil, fmt.Errorf("scanning inbound row: %w", err)
		}
		nb.sourceID = nb.id // actual src_note_id from DB, not BFS parent
```

Remove the duplicate error return that was inside the old block (keep the one after Scan).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestTraverse_InboundEdgeSourceID ./internal/graph/...`
Expected: PASS

- [ ] **Step 5: Run full graph test suite**

Run: `go test -v ./internal/graph/...`
Expected: All tests pass (existing tests for outbound edges should be unaffected)

- [ ] **Step 6: Commit**

```bash
git add internal/graph/traverse.go internal/graph/traverse_test.go
git commit -m "fix(graph): correct inbound edge sourceID in BFS traversal

TraverseEdge.SourceID for inbound-discovered nodes was set to the BFS
parent (current node) instead of the actual src_note_id from the links
table. This inverted edge direction for all inbound edges, causing
memory recall and links neighbors to report wrong source IDs."
```

---

### Task 2: Add goose v3 schema migration

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/index/migrations/001_baseline_schema.sql`
- Modify: `internal/index/db.go`
- Modify: `internal/index/db_test.go` (if exists, otherwise tests in existing test files)

- [ ] **Step 1: Add goose dependency**

```bash
go get github.com/pressly/goose/v3
```

- [ ] **Step 2: Verify license compliance**

```bash
task check:license:source
```

Expected: PASS (goose is MIT, transitive deps go-retry and multierr are MIT)

- [ ] **Step 3: Write the failing test for migration**

Add to a test file in `internal/index/` (use existing test patterns):

```go
func TestMigrations_FreshDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// goose_db_version table should exist after Open
	var version int64
	err = db.QueryRow("SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1").Scan(&version)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, version, int64(1), "migration 001 should be applied")

	// All core tables should exist
	tables := []string{"notes", "aliases", "tags", "frontmatter_kv", "links", "blocks", "headings", "fts_notes", "generated_sections"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name = ?", tbl).Scan(&name)
		require.NoError(t, err, "table %s should exist", tbl)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test -v -run TestMigrations_FreshDB ./internal/index/...`
Expected: FAIL — goose_db_version table does not exist (current code uses applySchema, not goose)

- [ ] **Step 5: Create migration 001 — baseline schema**

Create `internal/index/migrations/001_baseline_schema.sql` with the full current schema extracted from `applySchema()`:

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS notes (
    rowid     INTEGER PRIMARY KEY AUTOINCREMENT,
    id        TEXT NOT NULL UNIQUE,
    path      TEXT NOT NULL UNIQUE,
    title     TEXT,
    type      TEXT,
    status    TEXT,
    created   TEXT,
    updated   TEXT,
    body_text TEXT,
    hash      TEXT NOT NULL,
    mtime     INTEGER NOT NULL,
    is_domain BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(type);

CREATE TABLE IF NOT EXISTS aliases (
    note_id          TEXT NOT NULL REFERENCES notes(id),
    alias            TEXT NOT NULL,
    alias_normalized TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aliases_normalized ON aliases(alias_normalized);
CREATE INDEX IF NOT EXISTS idx_aliases_note       ON aliases(note_id);

CREATE TABLE IF NOT EXISTS tags (
    note_id TEXT NOT NULL REFERENCES notes(id),
    tag     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);

CREATE TABLE IF NOT EXISTS frontmatter_kv (
    note_id    TEXT NOT NULL REFERENCES notes(id),
    key        TEXT NOT NULL,
    value_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_fmkv_note ON frontmatter_kv(note_id);

CREATE TABLE IF NOT EXISTS links (
    src_note_id TEXT NOT NULL,
    dst_note_id TEXT,
    dst_raw     TEXT NOT NULL,
    edge_type   TEXT NOT NULL,
    target_kind TEXT,
    heading     TEXT,
    block_id    TEXT,
    resolved    BOOLEAN NOT NULL DEFAULT FALSE,
    confidence  TEXT NOT NULL DEFAULT 'high',
    origin      TEXT,
    weight      REAL
);
CREATE INDEX IF NOT EXISTS idx_links_src          ON links(src_note_id);
CREATE INDEX IF NOT EXISTS idx_links_dst          ON links(dst_note_id);
CREATE INDEX IF NOT EXISTS idx_links_edge_type    ON links(edge_type);
CREATE INDEX IF NOT EXISTS idx_links_confidence   ON links(confidence);
CREATE INDEX IF NOT EXISTS idx_links_src_resolved ON links(src_note_id, resolved);
CREATE UNIQUE INDEX IF NOT EXISTS idx_links_unique ON links(src_note_id, dst_note_id, edge_type, dst_raw);

CREATE TABLE IF NOT EXISTS blocks (
    note_id    TEXT NOT NULL REFERENCES notes(id),
    block_id   TEXT NOT NULL,
    heading    TEXT,
    start_line INTEGER NOT NULL,
    end_line   INTEGER
);

CREATE TABLE IF NOT EXISTS headings (
    note_id      TEXT NOT NULL REFERENCES notes(id),
    heading_slug TEXT NOT NULL,
    level        INTEGER NOT NULL,
    title        TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_notes USING fts5(
    note_id UNINDEXED,
    title,
    body_text
);

CREATE TABLE IF NOT EXISTS generated_sections (
    note_id     TEXT NOT NULL REFERENCES notes(id),
    section_key TEXT NOT NULL,
    checksum    TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (note_id, section_key)
);
```

- [ ] **Step 6: Replace applySchema with goose migrations in db.go**

Edit `internal/index/db.go`:

Add imports:
```go
import (
    "context"
    "embed"
    // ... existing imports ...
    "github.com/pressly/goose/v3"
)
```

Add embed directive after the imports/before the DB type:
```go
//go:embed migrations/*.sql
var migrations embed.FS
```

Replace the `applySchema()` method with:

```go
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

In the `Open()` function, replace the call from `d.applySchema()` to `d.applyMigrations()`. Keep the pragma block — pragmas must run before migrations. Move pragma execution into its own method or keep inline:

```go
func (d *DB) applyPragmas() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := d.db.Exec(p); err != nil {
			return fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	return nil
}
```

Update `Open()`:
```go
d := &DB{db: sqlDB}
if err := d.applyPragmas(); err != nil {
    _ = sqlDB.Close()
    return nil, fmt.Errorf("applying pragmas: %w", err)
}
if err := d.applyMigrations(); err != nil {
    _ = sqlDB.Close()
    return nil, fmt.Errorf("applying migrations: %w", err)
}
return d, nil
```

Remove the old `applySchema()` method entirely.

- [ ] **Step 7: Run test to verify it passes**

Run: `go test -v -run TestMigrations_FreshDB ./internal/index/...`
Expected: PASS

- [ ] **Step 8: Run full index test suite**

Run: `task test`
Expected: All 1807+ tests pass. Existing DBs get migration 001 applied (IF NOT EXISTS makes it safe).

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/index/db.go internal/index/migrations/001_baseline_schema.sql internal/index/db_test.go
git commit -m "feat(index): add goose v3 schema migration

Replace monolithic applySchema() with goose-managed migrations.
Migration 001 contains the full baseline schema (identical DDL).
Existing databases migrate safely via IF NOT EXISTS statements.
Pragmas (WAL, foreign_keys) run before migrations."
```

---

### Task 3: Add COLLATE NOCASE indexes (migration 002)

**Files:**
- Create: `internal/index/migrations/002_add_title_alias_indexes.sql`
- Modify: `internal/index/db_test.go`

- [ ] **Step 1: Update TestOpen_CreatesAllIndexes to expect the new indexes**

In `internal/index/db_test.go`, find `TestOpen_CreatesAllIndexes` (line ~55) and add the two new index names to the `expectedIndexes` slice:

```go
expectedIndexes := []string{
    "idx_aliases_normalized",
    "idx_tags_tag",
    "idx_fmkv_note",
    "idx_links_src",
    "idx_links_dst",
    "idx_links_edge_type",
    "idx_links_confidence",
    "idx_links_src_resolved",
    "idx_links_unique",
    "idx_notes_type",
    "idx_aliases_note",
    "idx_notes_title",    // new: migration 002
    "idx_aliases_alias",  // new: migration 002
}
```

Also add a dedicated test:

```go
func TestMigrations_TitleAliasIndexes(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	indexes := []string{"idx_notes_title", "idx_aliases_alias"}
	for _, idx := range indexes {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name = ?", idx).Scan(&name)
		require.NoError(t, err, "index %s should exist", idx)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestMigrations_TitleAliasIndexes ./internal/index/...`
Expected: FAIL — indexes don't exist yet

- [ ] **Step 3: Create migration 002**

Create `internal/index/migrations/002_add_title_alias_indexes.sql`:

```sql
-- +goose Up
CREATE INDEX IF NOT EXISTS idx_notes_title ON notes(title COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_aliases_alias ON aliases(alias COLLATE NOCASE);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestMigrations_TitleAliasIndexes ./internal/index/...`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `task test`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/index/migrations/002_add_title_alias_indexes.sql internal/index/db_test.go
git commit -m "feat(index): add COLLATE NOCASE indexes on title and alias

Session 03 Finding #9/#14: ResolveLinks does case-insensitive lookups
on notes(title) and aliases(alias) without indexes — full table scans.
New indexes provide ~1500x improvement at 10K notes."
```

---

### Task 4: Normalize BM25 scores to 0–1

**Files:**
- Modify: `internal/index/fts.go:86-94`
- Modify: `internal/index/fts_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/index/fts_test.go`:

```go
func TestSearchFTS_ScoresNormalized(t *testing.T) {
	db := rebuildTestIndex(t)

	// "memory" matches multiple notes with varying relevance
	results, err := index.SearchFTS(db, "memory", 100, 0)
	require.NoError(t, err)
	require.Greater(t, len(results), 1, "need multiple results to test normalization")

	for _, r := range results {
		assert.GreaterOrEqual(t, r.Score, 0.0, "score should be >= 0")
		assert.LessOrEqual(t, r.Score, 1.0, "score should be <= 1")
	}

	// Best result should have score 1.0
	assert.Equal(t, 1.0, results[0].Score, "top result should have score 1.0")

	// If spread > 0, worst result should have score 0.0
	if len(results) > 1 {
		assert.Equal(t, 0.0, results[len(results)-1].Score, "worst result should have score 0.0")
	}
}

func TestSearchFTS_SingleResult_ScoreIsOne(t *testing.T) {
	db := rebuildTestIndex(t)

	// Use a very specific term that matches exactly one note
	results, err := index.SearchFTS(db, "Ebbinghaus", 100, 0)
	require.NoError(t, err)
	if len(results) == 1 {
		assert.Equal(t, 1.0, results[0].Score, "single result should have score 1.0")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestSearchFTS_ScoresNormalized ./internal/index/...`
Expected: FAIL — scores are raw negated BM25 floats, not in [0,1]

- [ ] **Step 3: Add min-max normalization to SearchFTS**

Edit `internal/index/fts.go`. After the existing score negation loop (around line 88-90) and before the `return results, rows.Err()` line, add:

```go
	// Normalize scores to [0, 1] via min-max scaling.
	if len(results) > 0 {
		minScore := results[0].Score
		maxScore := results[0].Score
		for _, r := range results[1:] {
			if r.Score < minScore {
				minScore = r.Score
			}
			if r.Score > maxScore {
				maxScore = r.Score
			}
		}
		spread := maxScore - minScore
		if spread > 0 {
			for i := range results {
				results[i].Score = (results[i].Score - minScore) / spread
			}
		} else {
			for i := range results {
				results[i].Score = 1.0
			}
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run TestSearchFTS_Scores ./internal/index/...`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `task test`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/index/fts.go internal/index/fts_test.go
git commit -m "feat(index): normalize BM25 scores to 0-1 range

Apply min-max normalization after negating raw BM25 scores.
Single-result queries get score 1.0. Multiple results span [0, 1].
Prerequisite for multi-signal retrieval blending in v2."
```

---

### Task 5: Add CountFTS for total document count

**Files:**
- Modify: `internal/index/fts.go`
- Modify: `internal/index/fts_test.go`
- Modify: `internal/query/search.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/index/fts_test.go`:

```go
func TestCountFTS_ReturnsTotal(t *testing.T) {
	db := rebuildTestIndex(t)

	// Get a broad query with many matches
	allResults, err := index.SearchFTS(db, "memory", 100, 0)
	require.NoError(t, err)
	totalExpected := len(allResults)
	require.Greater(t, totalExpected, 3, "need more than 3 results for this test")

	// CountFTS should return the total, regardless of limit
	count, err := index.CountFTS(db, "memory")
	require.NoError(t, err)
	assert.Equal(t, totalExpected, count, "CountFTS should return total matching docs")

	// SearchFTS with limit=2 returns only 2, but CountFTS still returns total
	limited, err := index.SearchFTS(db, "memory", 2, 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(limited), 2)

	count2, err := index.CountFTS(db, "memory")
	require.NoError(t, err)
	assert.Equal(t, totalExpected, count2, "CountFTS should be independent of limit")
}

func TestCountFTS_EmptyQuery(t *testing.T) {
	db := rebuildTestIndex(t)
	count, err := index.CountFTS(db, "")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestCountFTS_WithFilters(t *testing.T) {
	db := rebuildTestIndex(t)

	countAll, err := index.CountFTS(db, "memory")
	require.NoError(t, err)

	countFiltered, err := index.CountFTS(db, "memory", index.SearchFilters{Type: "concept"})
	require.NoError(t, err)
	assert.LessOrEqual(t, countFiltered, countAll, "filtered count should be <= total")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestCountFTS ./internal/index/...`
Expected: FAIL — `CountFTS` is undefined

- [ ] **Step 3: Implement CountFTS**

Add to `internal/index/fts.go`:

```go
// CountFTS returns the total number of documents matching the query and filters,
// independent of any limit/offset. Used for pagination totals.
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestCountFTS ./internal/index/...`
Expected: PASS

- [ ] **Step 5: Update RunSearch to use CountFTS**

Edit `internal/query/search.go`. In `RunSearch`, replace `Total: len(results)` with the count from `CountFTS`:

```go
func RunSearch(db *index.DB, cfg SearchConfig, w io.Writer) error {
	filters := index.SearchFilters{Type: cfg.TypeFilter, Tag: cfg.TagFilter}
	results, err := index.SearchFTS(db, cfg.Query, cfg.Limit, cfg.Offset, filters)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}

	if results == nil {
		results = []index.FTSResult{}
	}

	total, err := index.CountFTS(db, cfg.Query, filters)
	if err != nil {
		return fmt.Errorf("counting results: %w", err)
	}

	if cfg.JSONOutput {
		env := envelope.OK("search", SearchResult{
			Query: cfg.Query, Offset: cfg.Offset, Limit: cfg.Limit,
			Hits: results, Total: total,
		})
		env.Meta.VaultPath = cfg.VaultPath
		return json.NewEncoder(w).Encode(env)
	}

	for _, r := range results {
		if _, err := fmt.Fprintf(w, "%s  %s\n", r.ID, r.Title); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 6: Run full test suite**

Run: `task test`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add internal/index/fts.go internal/index/fts_test.go internal/query/search.go
git commit -m "fix(query): SearchResult.Total returns total doc count, not page count

Session 03 Finding #7/#15: Total was len(results) which returned the
page count (at most limit). Add CountFTS with same MATCH expression
and filters to return the true total for pagination."
```

---

### Task 6: Add Retriever interface and FTSRetriever

**Files:**
- Create: `internal/query/retriever.go`
- Create: `internal/query/fts_retriever.go`
- Create: `internal/query/fts_retriever_test.go`
- Modify: `internal/query/search.go`
- Modify: `cmd/search.go`

- [ ] **Step 1: Write the failing test**

Create `internal/query/fts_retriever_test.go`:

```go
package query_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check: FTSRetriever satisfies Retriever interface
var _ query.Retriever = (*query.FTSRetriever)(nil)

func buildRetrieverTestDB(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	vaultPath := "../../vaultmind-vault"

	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFTSRetriever_Search(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}

	results, total, err := retriever.Search(context.Background(), "memory", 5, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 5)
	assert.GreaterOrEqual(t, total, len(results), "total should be >= page results")

	for _, r := range results {
		assert.NotEmpty(t, r.ID)
		assert.GreaterOrEqual(t, r.Score, 0.0)
		assert.LessOrEqual(t, r.Score, 1.0)
	}
}

func TestFTSRetriever_SearchEmpty(t *testing.T) {
	db := buildRetrieverTestDB(t)
	retriever := &query.FTSRetriever{DB: db}

	results, total, err := retriever.Search(context.Background(), "", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Equal(t, 0, total)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestFTSRetriever ./internal/query/...`
Expected: FAIL — `query.Retriever` and `query.FTSRetriever` undefined

- [ ] **Step 3: Create the Retriever interface**

Create `internal/query/retriever.go`:

```go
package query

import (
	"context"

	"github.com/peiman/vaultmind/internal/index"
)

// ScoredResult is a retrieval hit with a normalized score in [0, 1].
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
// Scores are normalized to [0, 1]. Total is the full matching count
// regardless of limit/offset.
type Retriever interface {
	Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error)
}
```

- [ ] **Step 4: Create the FTSRetriever implementation**

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

// Search runs FTS5 search and count, converting results to ScoredResult.
func (r *FTSRetriever) Search(_ context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	results, err := index.SearchFTS(r.DB, query, limit, offset, filters)
	if err != nil {
		return nil, 0, err
	}

	total, err := index.CountFTS(r.DB, query, filters)
	if err != nil {
		return nil, 0, err
	}

	scored := make([]ScoredResult, len(results))
	for i, fr := range results {
		scored[i] = ScoredResult{
			ID:       fr.ID,
			Type:     fr.Type,
			Title:    fr.Title,
			Path:     fr.Path,
			Snippet:  fr.Snippet,
			Score:    fr.Score,
			IsDomain: fr.IsDomain,
		}
	}
	return scored, total, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v -run TestFTSRetriever ./internal/query/...`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `task test`
Expected: All tests pass. `RunSearch` and `cmd/search.go` are NOT changed yet — the Retriever exists alongside the direct call path.

- [ ] **Step 7: Commit**

```bash
git add internal/query/retriever.go internal/query/fts_retriever.go internal/query/fts_retriever_test.go
git commit -m "feat(query): add Retriever interface and FTSRetriever

Session 03 Finding #18: SearchFTS was called directly with no
abstraction. Define Retriever interface with normalized [0,1] scores
and total count. FTSRetriever wraps existing SearchFTS + CountFTS.
Prerequisite for v2 embedding-based retrieval."
```

---

### Task 7: Populate meta.index_hash in JSON envelopes

**Files:**
- Modify: `internal/cmdutil/helpers.go`
- Modify: `internal/cmdutil/helpers_test.go`
- Modify: 15 `cmd/*.go` files that build envelopes with `envelope.OK()`

- [ ] **Step 1: Write the failing test**

Add to `internal/cmdutil/helpers_test.go`:

```go
func TestVaultDB_GetIndexHash(t *testing.T) {
	vdb, err := cmdutil.OpenVaultDB("../../vaultmind-vault")
	require.NoError(t, err)
	defer vdb.Close()

	hash := vdb.GetIndexHash()
	assert.NotEmpty(t, hash, "index hash should not be empty")
	assert.Len(t, hash, 64, "SHA-256 hex should be 64 chars")

	// Should be consistent (cached)
	hash2 := vdb.GetIndexHash()
	assert.Equal(t, hash, hash2, "hash should be cached")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestVaultDB_GetIndexHash ./internal/cmdutil/...`
Expected: FAIL — `GetIndexHash` is undefined

- [ ] **Step 3: Add indexHash field and GetIndexHash to VaultDB**

Edit `internal/cmdutil/helpers.go`:

Add `indexHash` field to VaultDB:
```go
type VaultDB struct {
	DB        *index.DB
	Config    *vault.Config
	Reg       *schema.Registry
	dbPath    string
	indexHash string
}
```

Add getter:
```go
// GetIndexHash returns the cached SHA-256 hash of the index DB file.
func (v *VaultDB) GetIndexHash() string {
	return v.indexHash
}
```

In `OpenVaultDB`, after creating the VaultDB, compute and cache the hash:
```go
vdb := &VaultDB{
    DB:     db,
    Config: cfg,
    Reg:    schema.NewRegistry(cfg.Types),
    dbPath: dbPath,
}
vdb.indexHash = vdb.IndexHash()
return vdb, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestVaultDB_GetIndexHash ./internal/cmdutil/...`
Expected: PASS

- [ ] **Step 5: Add IndexHash to all envelope.OK calls in cmd files**

For each `cmd/*.go` file that builds envelopes with `envelope.OK()` and has a `vdb` variable, add `env.Meta.IndexHash = vdb.GetIndexHash()` after `env.Meta.VaultPath = vaultPath`.

Files to update (add `env.Meta.IndexHash = vdb.GetIndexHash()` line):
- `cmd/links_neighbors.go` — after line 47
- `cmd/note_mget.go` — after line 53
- `cmd/frontmatter_helpers.go` — after line 77
- `cmd/vault_status.go` — after line 37
- `cmd/apply.go` — after line 91
- `cmd/memory_related.go` — after line 50
- `cmd/frontmatter_validate.go` — after line 37
- `cmd/note_create.go` — after line 170
- `cmd/memory_context_pack.go` — after line 50
- `cmd/doctor.go` — after line 36
- `cmd/dataview_lint.go` — after line 89
- `cmd/memory_recall.go` — after line 52
- `cmd/links_in.go` — after the envelope line
- `cmd/links_out.go` — after the envelope line
- `cmd/note_get.go` — this one uses `query.RunNoteGet` which builds its own envelope, so handle in query layer
- `cmd/resolve.go` — same, uses `query.RunResolve`
- `cmd/search.go` — same, uses `query.RunSearch`
- `cmd/dataview_render.go` — uses `cmdutil.WriteJSON`, update signature

For commands that delegate to `query.Run*` functions (note_get, resolve, search), pass the indexHash as a field in the config struct, and set it on the envelope inside the query function.

The pattern for each cmd file is mechanical:
```go
// Before:
env := envelope.OK("command", result)
env.Meta.VaultPath = vaultPath

// After:
env := envelope.OK("command", result)
env.Meta.VaultPath = vaultPath
env.Meta.IndexHash = vdb.GetIndexHash()
```

- [ ] **Step 6: Run full test suite**

Run: `task test`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add internal/cmdutil/helpers.go internal/cmdutil/helpers_test.go cmd/*.go internal/query/*.go
git commit -m "feat(envelope): populate meta.index_hash in all JSON responses

Session 03 Finding #10: index_hash was defined but never populated.
Compute SHA-256 of the index DB once on OpenVaultDB, cache it, and
set env.Meta.IndexHash in all envelope.OK() calls. Agents can use
this for cache invalidation."
```

---

### Task 8: Route OpenVaultDB errors through JSON envelope

**Files:**
- Modify: `internal/cmdutil/helpers.go`
- Modify: `internal/cmdutil/helpers_test.go`
- Modify: 18 `cmd/*.go` files that call `OpenVaultDB`

- [ ] **Step 1: Write the failing test**

Add to `internal/cmdutil/helpers_test.go`:

```go
func TestOpenVaultDBOrWriteErr_JSONOutput(t *testing.T) {
	// Create a cobra command with --json flag
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", true, "")

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, "/nonexistent/path", "test-command")
	assert.Nil(t, vdb)
	require.Error(t, err)
	assert.True(t, errors.Is(err, cmdutil.ErrAlreadyWritten))

	// Verify JSON error envelope was written
	var env envelope.Envelope
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	assert.Equal(t, "test-command", env.Command)
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "vault_error", env.Errors[0].Code)
}

func TestOpenVaultDBOrWriteErr_TextOutput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, "/nonexistent/path", "test-command")
	assert.Nil(t, vdb)
	require.Error(t, err)
	assert.False(t, errors.Is(err, cmdutil.ErrAlreadyWritten))
	assert.Contains(t, err.Error(), "does not exist")
}
```

Add required imports to the test file: `bytes`, `encoding/json`, `errors`, `github.com/spf13/cobra`, `github.com/peiman/vaultmind/internal/envelope`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestOpenVaultDBOrWriteErr ./internal/cmdutil/...`
Expected: FAIL — `OpenVaultDBOrWriteErr` and `ErrAlreadyWritten` are undefined

- [ ] **Step 3: Implement OpenVaultDBOrWriteErr**

Edit `internal/cmdutil/helpers.go`. Add imports for `errors` (if not present) and `github.com/spf13/cobra`. Add:

```go
// ErrAlreadyWritten signals that a JSON error envelope was already written
// to the command output. The caller should return nil to avoid double-printing.
var ErrAlreadyWritten = errors.New("error already written to output")

// isJSONOutput checks whether the --json flag is set on the command.
func isJSONOutput(cmd *cobra.Command) bool {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	return jsonFlag
}

// OpenVaultDBOrWriteErr opens the vault DB. On failure, if --json is set,
// it writes a JSON error envelope and returns ErrAlreadyWritten.
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestOpenVaultDBOrWriteErr ./internal/cmdutil/...`
Expected: PASS

- [ ] **Step 5: Update all 18 cmd files to use OpenVaultDBOrWriteErr**

For each `cmd/*.go` file that calls `cmdutil.OpenVaultDB`, replace:

```go
vdb, err := cmdutil.OpenVaultDB(vaultPath)
if err != nil {
    return err
}
```

With:

```go
vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "COMMAND_NAME")
if err != nil {
    if errors.Is(err, cmdutil.ErrAlreadyWritten) {
        return nil
    }
    return err
}
```

Add `"errors"` to the import block of each file.

The 18 files and their command names:
1. `cmd/search.go` — `"search"`
2. `cmd/resolve.go` — `"resolve"`
3. `cmd/note_get.go` — `"note get"`
4. `cmd/note_mget.go` — `"note mget"`
5. `cmd/note_create.go` — `"note create"`
6. `cmd/links_in.go` — `"links in"`
7. `cmd/links_out.go` — `"links out"`
8. `cmd/links_neighbors.go` — `"links neighbors"`
9. `cmd/memory_recall.go` — `"memory recall"`
10. `cmd/memory_related.go` — `"memory related"`
11. `cmd/memory_context_pack.go` — `"memory context-pack"`
12. `cmd/doctor.go` — `"doctor"`
13. `cmd/vault_status.go` — `"vault status"`
14. `cmd/frontmatter_helpers.go` — `"frontmatter"` (shared by set/unset/merge/normalize)
15. `cmd/frontmatter_validate.go` — `"frontmatter validate"`
16. `cmd/dataview_lint.go` — `"dataview lint"`
17. `cmd/dataview_render.go` — `"dataview render"`
18. `cmd/apply.go` — `"apply"`

- [ ] **Step 6: Run full test suite**

Run: `task test`
Expected: All tests pass

- [ ] **Step 7: Run task check**

Run: `task check`
Expected: All 23 checks pass, coverage >= 85.5%

- [ ] **Step 8: Commit**

```bash
git add internal/cmdutil/helpers.go internal/cmdutil/helpers_test.go cmd/*.go
git commit -m "fix(cmd): route OpenVaultDB errors through JSON envelope

Session 03 Finding #6/#9: vault path, config, and database errors
bypassed the JSON envelope on all 24 --json commands. Agents parsing
JSON would crash on these errors. Add OpenVaultDBOrWriteErr helper
that writes a structured {code: 'vault_error'} envelope when --json
is set, with ErrAlreadyWritten sentinel for callers."
```

---

## Final Verification

After all 8 tasks are complete:

- [ ] **Run full quality check**

```bash
task check
```

Expected: All 23 checks pass, coverage >= 85.5%

- [ ] **Verify acceptance criteria**

Test each manually:

1. BFS edge direction: `go test -v -run TestTraverse_InboundEdgeSourceID ./internal/graph/...`
2. JSON envelope on vault error: `go build ./... && ./vaultmind search --json --vault /nonexistent test` → should return JSON `{"status":"error",...}`
3. SearchResult.Total: `go test -v -run TestCountFTS ./internal/index/...`
4. COLLATE NOCASE indexes: `go test -v -run TestMigrations_TitleAliasIndexes ./internal/index/...`
5. index_hash populated: `go test -v -run TestVaultDB_GetIndexHash ./internal/cmdutil/...`
6. BM25 normalized: `go test -v -run TestSearchFTS_ScoresNormalized ./internal/index/...`
7. Retriever interface: `go test -v -run TestFTSRetriever ./internal/query/...`
8. Goose migration: `go test -v -run TestMigrations ./internal/index/...`
