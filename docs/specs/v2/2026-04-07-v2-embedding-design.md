# v2 Embedding Retrieval — Design Spec

**Date:** 2026-04-07 (revised 2026-04-07)
**Goal:** Add embedding-based semantic search and N-way hybrid retrieval to VaultMind.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Model for steps 1-3 | all-MiniLM-L6-v2 (384 dims) | Proven with HugotEmbedder, no CGO, ship pipeline first |
| BGE-M3 upgrade | Step 4, separate effort | 3-in-1 (dense+sparse+ColBERT) changes retrieval architecture |
| Vector search | Brute-force cosine in Go | 10K notes × 384 dims = ~15MB, milliseconds to scan |
| BLOB format | Raw little-endian float32 bytes | Fastest encode/decode, 1,536 bytes per note at 384 dims |
| Score fusion | Pure RRF (rank-based, K=60) | Robust across different scoring scales, no normalization needed |
| Embed trigger | `--embed` flag on index command | Opt-in, no surprise model downloads |
| Hybrid combinator | Generic N-way RRF | BGE-M3's sparse/ColBERT become additional retrievers later |

## Architecture

```
index --embed:
  notes table ──→ body_text ──→ Embedder.EmbedBatch() ──→ []float32 ──→ BLOB ──→ notes.embedding

search --mode hybrid:
  query ──→ FTSRetriever ─────────────────────────────────┐
  query ──→ Embedder.Embed() ──→ EmbeddingRetriever ──────┤
                                                           ├──→ HybridRetriever (N-way RRF) ──→ results
  [future: BGE-M3 sparse retriever] ──────────────────────┤
  [future: BGE-M3 ColBERT reranker] ──────────────────────┘
```

## Step 1: Embedding Storage (`internal/index/embedding.go`)

Pure encode/decode helpers plus DB operations for the existing `notes.embedding` BLOB column (migration 003).

```go
func EncodeEmbedding(vec []float32) []byte
func DecodeEmbedding(data []byte) ([]float32, error)
func StoreEmbedding(d *DB, noteID string, vec []float32) error
func LoadEmbedding(d *DB, noteID string) ([]float32, error)
func LoadAllEmbeddings(d *DB) ([]NoteEmbedding, error)
func HasEmbeddings(d *DB) (bool, error)

type NoteEmbedding struct {
    NoteID    string
    Embedding []float32
}
```

- `EncodeEmbedding`: `binary.LittleEndian` write of `[]float32` to `[]byte`
- `DecodeEmbedding`: reverse; returns error if `len(data) % 4 != 0`
- `StoreEmbedding`: `UPDATE notes SET embedding = ? WHERE id = ?`
- `LoadEmbedding`: `SELECT embedding FROM notes WHERE id = ?`; returns `nil, nil` if NULL
- `LoadAllEmbeddings`: `SELECT id, embedding FROM notes WHERE embedding IS NOT NULL`
- `HasEmbeddings`: `SELECT 1 FROM notes WHERE embedding IS NOT NULL LIMIT 1`

## Step 2: Index `--embed` Flag

**Command layer** (`cmd/index.go`): Add `--embed` bool flag. If set, create an Embedder and pass it to the indexer. Stays within the ≤30 line budget by delegating to internal.

**Indexer** (`internal/index/indexer.go`): New method after Rebuild/Incremental:

```go
func (idx *Indexer) EmbedNotes(ctx context.Context, embedder embedding.Embedder) (*EmbedResult, error)

type EmbedResult struct {
    Embedded int
    Skipped  int
    Errors   int
}
```

**Flow:**
1. `SELECT id, body_text FROM notes WHERE embedding IS NULL` — only unembedded notes
2. Batch body texts through `embedder.EmbedBatch()` in chunks of 32 (keeps memory bounded)
3. `StoreEmbedding()` for each result within a transaction per batch
4. Return stats

**Incremental awareness:** The existing delete-before-reinsert pattern in `StoreNoteInTx` nulls the embedding column when a note's content hash changes. The embed pass then re-embeds it via the `WHERE embedding IS NULL` query. No extra invalidation logic needed.

**Output:**
```
Indexed 129 notes (0 errors)
Embedded 129 notes (0 skipped, 0 errors)
```

## Step 3: EmbeddingRetriever (`internal/query/embedding_retriever.go`)

Implements the existing `Retriever` interface using brute-force cosine similarity.

```go
type EmbeddingRetriever struct {
    DB       *index.DB
    Embedder embedding.Embedder
}
```

**Search flow:**
1. `embedder.Embed(ctx, query)` — embed the query string
2. `index.LoadAllEmbeddings(db)` — load all stored vectors
3. Cosine similarity between query vector and each note vector
4. Sort descending by score
5. Filter by `SearchFilters` (type, tag) — requires joining note metadata
6. Apply offset/limit
7. Return `[]ScoredResult` with cosine as score (naturally [0, 1] for normalized vectors)

**Cosine similarity** (`internal/query/similarity.go`):
```go
func CosineSimilarity(a, b []float32) float64
```
Dot product / (magnitude_a × magnitude_b). Pure function, independently testable.

## Step 4: N-Way RRF HybridRetriever (`internal/query/hybrid_retriever.go`)

Generic rank fusion combinator over any N retrievers.

```go
type HybridRetriever struct {
    Retrievers []Retriever
    K          int // RRF constant, default 60
}
```

**Search flow:**
1. Call `Search()` on each retriever concurrently via `errgroup`
2. For each result, compute `1.0 / (K + rank)` where rank is 1-based
3. Sum RRF scores per note ID across all retrievers
4. Sort by combined RRF score descending
5. Apply offset/limit
6. Return merged `[]ScoredResult` with RRF score

**Error handling:** If any retriever errors, the whole search errors. No silent degradation.

**Degenerate cases:**
- Zero results from a retriever — contributes nothing (correct)
- Single retriever — monotonic transform of original ranking (correct)
- Same note in all retrievers — gets highest combined score (correct)

## Step 5: Command Integration

**`cmd/search.go`** — add `--mode` flag:
- `keyword` (default): `FTSRetriever` — current behavior, unchanged
- `semantic`: `EmbeddingRetriever` — requires embeddings, errors if none exist
- `hybrid`: `HybridRetriever{[FTS, Embedding], K: 60}`

`semantic` and `hybrid` error with: "No embeddings found. Run `vaultmind index --embed` first."

**`cmd/ask.go`** — auto-detect:
- If `HasEmbeddings(db)` is true → use `hybrid` mode
- Otherwise → use `keyword` mode (current behavior)
- No flag needed — `ask` is the "just work" command

**Embedder lifecycle:**
- Created in command layer when needed (semantic/hybrid mode, or `--embed` flag)
- `defer embedder.Close()`
- Not loaded for keyword-only operations

## BGE-M3 Upgrade Path (Future — Step 4 of Roadmap)

BGE-M3 (568M, MIT, 1024 dims) produces three vector types from one model:
- **Dense** — standard embedding (replaces all-MiniLM-L6-v2 in `EmbeddingRetriever`)
- **Sparse** — learned sparse vectors (could replace or complement SQLite FTS)
- **ColBERT** — token-level late interaction (reranking)

Each becomes its own `Retriever` implementation fed into the same `HybridRetriever` combinator. The N-way RRF design accommodates this without rearchitecting.

Requires hugot ORT backend (CGO + ship `libonnxruntime`). Separate design spec.

## Testing Strategy

- **Step 1:** Round-trip encode/decode property tests, store/load against temp DB
- **Step 2:** Index with `--embed` against test fixtures, verify BLOBs populated, verify incremental re-embeds on content change
- **Step 3:** EmbeddingRetriever with known vectors in temp DB, verify ranking order, verify filters
- **Step 4:** HybridRetriever with mock retrievers returning known ranked lists, verify RRF math, verify concurrent execution, verify degenerate cases
- **Step 5:** Integration tests for `--mode` flag behavior and `ask` auto-detection

Embedding-heavy tests (steps 2-3 with real models) gated behind `VAULTMIND_TEST_EMBEDDING=1`.
