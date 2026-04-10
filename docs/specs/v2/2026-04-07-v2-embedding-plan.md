# v2 Embedding Retrieval — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add embedding-based semantic search and N-way hybrid retrieval to VaultMind, using the proven all-MiniLM-L6-v2 model via hugot.

**Architecture:** Bottom-up pipeline — embedding storage helpers, then index `--embed` flag, then EmbeddingRetriever (brute-force cosine), then generic N-way RRF HybridRetriever, then command wiring (`search --mode`, `ask` auto-detection). Each task is independently testable and committable.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), hugot (knights-analytics/hugot), testify, cobra/viper config registry

**Spec:** `docs/specs/2026-04-07-v2-embedding-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/index/embedding.go` | Create | Encode/decode `[]float32` ↔ BLOB, store/load/query embeddings |
| `internal/index/embedding_test.go` | Create | Round-trip, store/load, HasEmbeddings tests |
| `internal/index/indexer.go` | Modify | Add `EmbedNotes()` method |
| `internal/index/indexer_test.go` | Modify | Add embedding integration tests |
| `internal/query/similarity.go` | Create | `CosineSimilarity(a, b []float32) float64` |
| `internal/query/similarity_test.go` | Create | Cosine math tests |
| `internal/query/embedding_retriever.go` | Create | Brute-force cosine search, implements `Retriever` |
| `internal/query/embedding_retriever_test.go` | Create | Retriever tests with known vectors |
| `internal/query/hybrid_retriever.go` | Create | N-way RRF combinator |
| `internal/query/hybrid_retriever_test.go` | Create | RRF math, concurrency, degenerate cases |
| `internal/config/commands/index_config.go` | Modify | Add `app.index.embed` option |
| `internal/config/commands/search_config.go` | Modify | Add `app.search.mode` option |
| `cmd/index.go` | Modify | Wire `--embed` flag to `EmbedNotes()` |
| `cmd/search.go` | Modify | Wire `--mode` flag to select retriever |
| `cmd/ask.go` | Modify | Auto-detect hybrid mode |

---

### Task 1: Embedding Encode/Decode Helpers

**Files:**
- Create: `internal/index/embedding.go`
- Create: `internal/index/embedding_test.go`

- [ ] **Step 1: Write failing tests for EncodeEmbedding / DecodeEmbedding**

```go
// internal/index/embedding_test.go
package index_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeEmbedding_RoundTrip(t *testing.T) {
	original := []float32{0.1, -0.5, 3.14, 0, -1e-6}
	encoded := index.EncodeEmbedding(original)
	assert.Len(t, encoded, len(original)*4, "each float32 is 4 bytes")

	decoded, err := index.DecodeEmbedding(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeEmbedding_InvalidLength(t *testing.T) {
	_, err := index.DecodeEmbedding([]byte{0x01, 0x02, 0x03}) // 3 bytes, not divisible by 4
	assert.Error(t, err)
}

func TestEncodeEmbedding_Empty(t *testing.T) {
	encoded := index.EncodeEmbedding(nil)
	assert.Nil(t, encoded)
}

func TestDecodeEmbedding_Empty(t *testing.T) {
	decoded, err := index.DecodeEmbedding(nil)
	require.NoError(t, err)
	assert.Nil(t, decoded)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run TestEncodeDecodeEmbedding ./internal/index/...`
Expected: FAIL — `EncodeEmbedding` undefined

- [ ] **Step 3: Implement EncodeEmbedding and DecodeEmbedding**

```go
// internal/index/embedding.go
package index

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// EncodeEmbedding serializes a float32 slice to raw little-endian bytes for BLOB storage.
func EncodeEmbedding(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// DecodeEmbedding deserializes raw little-endian bytes back to a float32 slice.
func DecodeEmbedding(data []byte) ([]float32, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding data: length %d not divisible by 4", len(data))
	}
	vec := make([]float32, len(data)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vec, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestEncodeDecodeEmbedding|TestDecodeEmbedding|TestEncodeEmbedding" ./internal/index/...`
Expected: PASS — all 4 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/index/embedding.go internal/index/embedding_test.go
git commit -m "feat(index): add embedding encode/decode helpers for BLOB storage"
```

---

### Task 2: Embedding Store/Load/Query DB Operations

**Files:**
- Modify: `internal/index/embedding.go`
- Modify: `internal/index/embedding_test.go`

- [ ] **Step 1: Write failing tests for StoreEmbedding, LoadEmbedding, LoadAllEmbeddings, HasEmbeddings**

Add to `internal/index/embedding_test.go`:

```go
func buildEmbeddingTestDB(t *testing.T) *index.DB {
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

func TestStoreAndLoadEmbedding(t *testing.T) {
	db := buildEmbeddingTestDB(t)

	// Pick an existing note ID from the indexed vault
	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row, "test vault must contain spreading-activation.md")

	vec := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	err = index.StoreEmbedding(db, row.ID, vec)
	require.NoError(t, err)

	loaded, err := index.LoadEmbedding(db, row.ID)
	require.NoError(t, err)
	assert.Equal(t, vec, loaded)
}

func TestLoadEmbedding_NoEmbedding(t *testing.T) {
	db := buildEmbeddingTestDB(t)
	loaded, err := index.LoadEmbedding(db, "concept-spreading-activation")
	require.NoError(t, err)
	assert.Nil(t, loaded, "note without embedding should return nil")
}

func TestLoadAllEmbeddings(t *testing.T) {
	db := buildEmbeddingTestDB(t)

	// Store embeddings for two notes
	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/knowledge-graph.md")
	require.NoError(t, err)
	require.NotNil(t, row2)

	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{0.1, 0.2}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0.3, 0.4}))

	all, err := index.LoadAllEmbeddings(db)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	ids := map[string]bool{all[0].NoteID: true, all[1].NoteID: true}
	assert.True(t, ids[row1.ID])
	assert.True(t, ids[row2.ID])
}

func TestHasEmbeddings(t *testing.T) {
	db := buildEmbeddingTestDB(t)

	has, err := index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.False(t, has, "fresh index has no embeddings")

	row, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NoError(t, index.StoreEmbedding(db, row.ID, []float32{0.1}))

	has, err = index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.True(t, has)
}
```

Also add these imports at the top of the test file:

```go
import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run "TestStoreAndLoad|TestLoadEmbedding_No|TestLoadAllEmbeddings|TestHasEmbeddings" ./internal/index/...`
Expected: FAIL — `StoreEmbedding` undefined

- [ ] **Step 3: Implement StoreEmbedding, LoadEmbedding, LoadAllEmbeddings, HasEmbeddings**

Add to `internal/index/embedding.go`:

```go
// NoteEmbedding pairs a note ID with its embedding vector.
type NoteEmbedding struct {
	NoteID    string
	Embedding []float32
}

// StoreEmbedding writes an embedding BLOB for a note that already exists in the index.
func StoreEmbedding(d *DB, noteID string, vec []float32) error {
	_, err := d.Exec("UPDATE notes SET embedding = ? WHERE id = ?", EncodeEmbedding(vec), noteID)
	if err != nil {
		return fmt.Errorf("storing embedding for %q: %w", noteID, err)
	}
	return nil
}

// LoadEmbedding reads the embedding for a single note. Returns nil, nil if no embedding stored.
func LoadEmbedding(d *DB, noteID string) ([]float32, error) {
	var data []byte
	err := d.QueryRow("SELECT embedding FROM notes WHERE id = ?", noteID).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("loading embedding for %q: %w", noteID, err)
	}
	if data == nil {
		return nil, nil
	}
	return DecodeEmbedding(data)
}

// LoadAllEmbeddings returns all notes that have stored embeddings.
func LoadAllEmbeddings(d *DB) ([]NoteEmbedding, error) {
	rows, err := d.Query("SELECT id, embedding FROM notes WHERE embedding IS NOT NULL")
	if err != nil {
		return nil, fmt.Errorf("loading all embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []NoteEmbedding
	for rows.Next() {
		var ne NoteEmbedding
		var data []byte
		if err := rows.Scan(&ne.NoteID, &data); err != nil {
			return nil, fmt.Errorf("scanning embedding row: %w", err)
		}
		vec, decErr := DecodeEmbedding(data)
		if decErr != nil {
			return nil, fmt.Errorf("decoding embedding for %q: %w", ne.NoteID, decErr)
		}
		ne.Embedding = vec
		result = append(result, ne)
	}
	return result, rows.Err()
}

// HasEmbeddings returns true if any note in the index has a stored embedding.
func HasEmbeddings(d *DB) (bool, error) {
	var exists int
	err := d.QueryRow("SELECT 1 FROM notes WHERE embedding IS NOT NULL LIMIT 1").Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("checking for embeddings: %w", err)
	}
	return true, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestStoreAndLoad|TestLoadEmbedding_No|TestLoadAllEmbeddings|TestHasEmbeddings" ./internal/index/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/index/embedding.go internal/index/embedding_test.go
git commit -m "feat(index): add embedding store/load/query DB operations"
```

---

### Task 3: EmbedNotes Method on Indexer

**Files:**
- Modify: `internal/index/indexer.go`
- Modify or create: `internal/index/indexer_test.go` (add embedding test)

- [ ] **Step 1: Write failing test for EmbedNotes**

Add to the indexer test file (create `internal/index/embed_test.go` if `indexer_test.go` doesn't have a good spot):

```go
// internal/index/embed_test.go
package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbedNotes(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_EMBEDDING") == "" {
		t.Skip("skipping embedding test (set VAULTMIND_TEST_EMBEDDING=1)")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	vaultPath := "../../vaultmind-vault"
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    "sentence-transformers/all-MiniLM-L6-v2",
		CacheDir:     t.TempDir(),
		Dims:         384,
		OnnxFilePath: "onnx/model.onnx",
	})
	require.NoError(t, err)
	defer func() { _ = embedder.Close() }()

	result, err := idxr.EmbedNotes(context.Background(), dbPath, embedder)
	require.NoError(t, err)
	assert.Greater(t, result.Embedded, 0, "should embed at least one note")
	assert.Equal(t, 0, result.Errors)

	// Verify embeddings were stored
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	has, err := index.HasEmbeddings(db)
	require.NoError(t, err)
	assert.True(t, has)

	all, err := index.LoadAllEmbeddings(db)
	require.NoError(t, err)
	assert.Equal(t, result.Embedded, len(all))
	for _, ne := range all {
		assert.Len(t, ne.Embedding, 384)
	}
}

func TestEmbedNotes_Incremental(t *testing.T) {
	if os.Getenv("VAULTMIND_TEST_EMBEDDING") == "" {
		t.Skip("skipping embedding test (set VAULTMIND_TEST_EMBEDDING=1)")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	vaultPath := "../../vaultmind-vault"
	cfg, err := vault.LoadConfig(vaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(vaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    "sentence-transformers/all-MiniLM-L6-v2",
		CacheDir:     t.TempDir(),
		Dims:         384,
		OnnxFilePath: "onnx/model.onnx",
	})
	require.NoError(t, err)
	defer func() { _ = embedder.Close() }()

	// First embed
	result1, err := idxr.EmbedNotes(context.Background(), dbPath, embedder)
	require.NoError(t, err)
	assert.Greater(t, result1.Embedded, 0)

	// Second embed — all should be skipped
	result2, err := idxr.EmbedNotes(context.Background(), dbPath, embedder)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Embedded)
	assert.Equal(t, result1.Embedded, result2.Skipped)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `VAULTMIND_TEST_EMBEDDING=1 go test -v -run TestEmbedNotes ./internal/index/...`
Expected: FAIL — `EmbedNotes` undefined

- [ ] **Step 3: Implement EmbedNotes**

Add to `internal/index/indexer.go`:

```go
// EmbedResult holds the outcome of an embedding pass.
type EmbedResult struct {
	Embedded int `json:"embedded"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

// EmbedNotes computes and stores embeddings for all notes that don't have one yet.
// It opens its own DB connection (like Rebuild/Incremental) so it can be called
// after the indexer has closed its connection.
func (idx *Indexer) EmbedNotes(ctx context.Context, dbPath string, embedder embedding.Embedder) (*EmbedResult, error) {
	db, err := Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening index for embedding: %w", err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT id, body_text FROM notes WHERE embedding IS NULL")
	if err != nil {
		return nil, fmt.Errorf("querying unembedded notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type noteText struct {
		id   string
		body string
	}
	var pending []noteText
	for rows.Next() {
		var nt noteText
		if err := rows.Scan(&nt.id, &nt.body); err != nil {
			return nil, fmt.Errorf("scanning note for embedding: %w", err)
		}
		pending = append(pending, nt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unembedded notes: %w", err)
	}

	result := &EmbedResult{}

	// Count notes that already have embeddings as skipped
	var totalNotes int
	if err := db.QueryRow("SELECT COUNT(*) FROM notes").Scan(&totalNotes); err != nil {
		return nil, fmt.Errorf("counting notes: %w", err)
	}
	result.Skipped = totalNotes - len(pending)

	// Process in batches of 32
	const batchSize = 32
	for i := 0; i < len(pending); i += batchSize {
		end := i + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[i:end]

		texts := make([]string, len(batch))
		for j, nt := range batch {
			texts[j] = nt.body
		}

		vecs, embErr := embedder.EmbedBatch(ctx, texts)
		if embErr != nil {
			log.Debug().Err(embErr).Int("batch_start", i).Msg("embedding batch failed")
			result.Errors += len(batch)
			continue
		}

		tx, txErr := db.Begin()
		if txErr != nil {
			return nil, fmt.Errorf("beginning embedding transaction: %w", txErr)
		}

		for j, vec := range vecs {
			encoded := EncodeEmbedding(vec)
			if _, err := tx.Exec("UPDATE notes SET embedding = ? WHERE id = ?", encoded, batch[j].id); err != nil {
				log.Debug().Err(err).Str("id", batch[j].id).Msg("storing embedding failed")
				result.Errors++
				continue
			}
			result.Embedded++
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing embedding batch: %w", err)
		}
	}

	return result, nil
}
```

Also add these imports to `indexer.go` (add to existing import block):

```go
"context"
"github.com/peiman/vaultmind/internal/embedding"
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `VAULTMIND_TEST_EMBEDDING=1 go test -v -run TestEmbedNotes ./internal/index/...`
Expected: PASS — both `TestEmbedNotes` and `TestEmbedNotes_Incremental` pass

- [ ] **Step 5: Run task check**

Run: `task check`
Expected: All checks pass

- [ ] **Step 6: Commit**

```bash
git add internal/index/indexer.go internal/index/embed_test.go
git commit -m "feat(index): add EmbedNotes method for batch embedding storage"
```

---

### Task 4: Cosine Similarity Function

**Files:**
- Create: `internal/query/similarity.go`
- Create: `internal/query/similarity_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/query/similarity_test.go
package query_test

import (
	"math"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1, 2, 3}
	sim := query.CosineSimilarity(a, a)
	assert.InDelta(t, 1.0, sim, 1e-6)
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, 0.0, sim, 1e-6)
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, -1.0, sim, 1e-6)
}

func TestCosineSimilarity_KnownValue(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{1, 1}
	// cos(45°) = 1/√2 ≈ 0.7071
	sim := query.CosineSimilarity(a, b)
	assert.InDelta(t, 1.0/math.Sqrt(2), sim, 1e-6)
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	sim := query.CosineSimilarity(a, b)
	assert.Equal(t, 0.0, sim)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run TestCosineSimilarity ./internal/query/...`
Expected: FAIL — `CosineSimilarity` undefined

- [ ] **Step 3: Implement CosineSimilarity**

```go
// internal/query/similarity.go
package query

import "math"

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns 0 if either vector has zero magnitude.
func CosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run TestCosineSimilarity ./internal/query/...`
Expected: PASS — all 5 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/query/similarity.go internal/query/similarity_test.go
git commit -m "feat(query): add CosineSimilarity function for vector search"
```

---

### Task 5: EmbeddingRetriever

**Files:**
- Create: `internal/query/embedding_retriever.go`
- Create: `internal/query/embedding_retriever_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/query/embedding_retriever_test.go
package query_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check
var _ query.Retriever = (*query.EmbeddingRetriever)(nil)

// mockEmbedder is a test double that returns a fixed vector for any input.
type mockEmbedder struct {
	vec  []float32
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.vec, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = m.vec
	}
	return result, nil
}

func (m *mockEmbedder) Dims() int   { return m.dims }
func (m *mockEmbedder) Close() error { return nil }

func TestEmbeddingRetriever_Search(t *testing.T) {
	db := buildRetrieverTestDB(t)

	// Store known embeddings for specific notes
	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/knowledge-graph.md")
	require.NoError(t, err)
	require.NotNil(t, row2)

	// row1 gets vector close to query, row2 gets orthogonal vector
	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0, 0}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0, 1, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0, 0}, dims: 3}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	results, total, err := retriever.Search(context.Background(), "spreading activation", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, results, 2)
	// row1 should rank first (identical vector = cosine 1.0)
	assert.Equal(t, row1.ID, results[0].ID)
	assert.InDelta(t, 1.0, results[0].Score, 1e-6)
	// row2 should rank second (orthogonal = cosine 0.0)
	assert.Equal(t, row2.ID, results[1].ID)
	assert.InDelta(t, 0.0, results[1].Score, 1e-6)
}

func TestEmbeddingRetriever_SearchWithLimit(t *testing.T) {
	db := buildRetrieverTestDB(t)

	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)
	row2, err := db.QueryNoteByPath("concepts/knowledge-graph.md")
	require.NoError(t, err)
	require.NotNil(t, row2)

	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0}))
	require.NoError(t, index.StoreEmbedding(db, row2.ID, []float32{0, 1}))

	embedder := &mockEmbedder{vec: []float32{1, 0}, dims: 2}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	results, total, err := retriever.Search(context.Background(), "test", 1, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total, "total should reflect all embeddings, not limit")
	assert.Len(t, results, 1, "limit=1 should return only 1 result")
}

func TestEmbeddingRetriever_SearchNoEmbeddings(t *testing.T) {
	db := buildRetrieverTestDB(t)

	embedder := &mockEmbedder{vec: []float32{1, 0}, dims: 2}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	results, total, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}

func TestEmbeddingRetriever_SearchWithTypeFilter(t *testing.T) {
	db := buildRetrieverTestDB(t)

	row1, err := db.QueryNoteByPath("concepts/spreading-activation.md")
	require.NoError(t, err)
	require.NotNil(t, row1)

	require.NoError(t, index.StoreEmbedding(db, row1.ID, []float32{1, 0}))

	embedder := &mockEmbedder{vec: []float32{1, 0}, dims: 2}
	retriever := &query.EmbeddingRetriever{DB: db, Embedder: embedder}

	// Filter by a type that doesn't match
	results, _, err := retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{Type: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, results)

	// Filter by correct type
	results, _, err = retriever.Search(context.Background(), "test", 10, 0, index.SearchFilters{Type: "concept"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run "TestEmbeddingRetriever" ./internal/query/...`
Expected: FAIL — `EmbeddingRetriever` undefined

- [ ] **Step 3: Implement EmbeddingRetriever**

```go
// internal/query/embedding_retriever.go
package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
)

// EmbeddingRetriever searches by cosine similarity between query and stored note embeddings.
type EmbeddingRetriever struct {
	DB       *index.DB
	Embedder embedding.Embedder
}

type scoredNote struct {
	noteID string
	score  float64
}

// Search embeds the query, computes cosine similarity against all stored embeddings,
// and returns the top results sorted by score descending.
func (r *EmbeddingRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	queryVec, err := r.Embedder.Embed(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("embedding query: %w", err)
	}

	all, err := index.LoadAllEmbeddings(r.DB)
	if err != nil {
		return nil, 0, fmt.Errorf("loading embeddings: %w", err)
	}
	if len(all) == 0 {
		return nil, 0, nil
	}

	// Score all notes
	scored := make([]scoredNote, len(all))
	for i, ne := range all {
		scored[i] = scoredNote{
			noteID: ne.NoteID,
			score:  CosineSimilarity(queryVec, ne.Embedding),
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Look up metadata and apply filters
	var filtered []ScoredResult
	for _, sn := range scored {
		row, err := r.DB.QueryNoteByID(sn.noteID)
		if err != nil || row == nil {
			continue
		}
		if filters.Type != "" && row.Type != filters.Type {
			continue
		}
		if filters.Tag != "" {
			hasTag, tagErr := r.noteHasTag(sn.noteID, filters.Tag)
			if tagErr != nil || !hasTag {
				continue
			}
		}
		filtered = append(filtered, ScoredResult{
			ID:       row.ID,
			Type:     row.Type,
			Title:    row.Title,
			Path:     row.Path,
			Score:    sn.score,
			IsDomain: row.IsDomain,
		})
	}

	total := len(filtered)

	// Apply offset/limit
	if offset >= len(filtered) {
		return nil, total, nil
	}
	filtered = filtered[offset:]
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, total, nil
}

func (r *EmbeddingRetriever) noteHasTag(noteID, tag string) (bool, error) {
	var count int
	err := r.DB.QueryRow("SELECT COUNT(*) FROM tags WHERE note_id = ? AND tag = ?", noteID, tag).Scan(&count)
	return count > 0, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestEmbeddingRetriever" ./internal/query/...`
Expected: PASS — all 4 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/query/embedding_retriever.go internal/query/embedding_retriever_test.go
git commit -m "feat(query): add EmbeddingRetriever with brute-force cosine search"
```

---

### Task 6: N-Way RRF HybridRetriever

**Files:**
- Create: `internal/query/hybrid_retriever.go`
- Create: `internal/query/hybrid_retriever_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/query/hybrid_retriever_test.go
package query_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check
var _ query.Retriever = (*query.HybridRetriever)(nil)

// staticRetriever returns a fixed set of results.
type staticRetriever struct {
	results []query.ScoredResult
	total   int
}

func (r *staticRetriever) Search(_ context.Context, _ string, limit, offset int, _ index.SearchFilters) ([]query.ScoredResult, int, error) {
	results := r.results
	if offset >= len(results) {
		return nil, r.total, nil
	}
	results = results[offset:]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, r.total, nil
}

func TestHybridRetriever_TwoRetrievers(t *testing.T) {
	// Retriever A ranks: note1, note2, note3
	retA := &staticRetriever{
		results: []query.ScoredResult{
			{ID: "note1", Title: "Note 1", Score: 1.0},
			{ID: "note2", Title: "Note 2", Score: 0.5},
			{ID: "note3", Title: "Note 3", Score: 0.1},
		},
		total: 3,
	}
	// Retriever B ranks: note2, note3, note1
	retB := &staticRetriever{
		results: []query.ScoredResult{
			{ID: "note2", Title: "Note 2", Score: 1.0},
			{ID: "note3", Title: "Note 3", Score: 0.5},
			{ID: "note1", Title: "Note 1", Score: 0.1},
		},
		total: 3,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []query.Retriever{retA, retB},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, results, 3)

	// note2 appears at rank 2 in A and rank 1 in B = 1/(60+2) + 1/(60+1) = highest RRF
	// note1 appears at rank 1 in A and rank 3 in B = 1/(60+1) + 1/(60+3)
	// note3 appears at rank 3 in A and rank 2 in B = 1/(60+3) + 1/(60+2)
	assert.Equal(t, "note2", results[0].ID, "note2 should rank first (best combined rank)")
	assert.Equal(t, "note1", results[1].ID, "note1 should rank second")
	assert.Equal(t, "note3", results[2].ID, "note3 should rank third")
}

func TestHybridRetriever_SingleRetriever(t *testing.T) {
	ret := &staticRetriever{
		results: []query.ScoredResult{
			{ID: "a", Title: "A", Score: 1.0},
			{ID: "b", Title: "B", Score: 0.5},
		},
		total: 2,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []query.Retriever{ret},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, "a", results[0].ID, "order should be preserved with single retriever")
	assert.Equal(t, "b", results[1].ID)
}

func TestHybridRetriever_EmptyRetriever(t *testing.T) {
	retA := &staticRetriever{
		results: []query.ScoredResult{
			{ID: "note1", Title: "Note 1", Score: 1.0},
		},
		total: 1,
	}
	retB := &staticRetriever{results: nil, total: 0}

	hybrid := &query.HybridRetriever{
		Retrievers: []query.Retriever{retA, retB},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "note1", results[0].ID)
}

func TestHybridRetriever_DisjointResults(t *testing.T) {
	retA := &staticRetriever{
		results: []query.ScoredResult{{ID: "a", Title: "A", Score: 1.0}},
		total:   1,
	}
	retB := &staticRetriever{
		results: []query.ScoredResult{{ID: "b", Title: "B", Score: 1.0}},
		total:   1,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []query.Retriever{retA, retB},
		K:          60,
	}

	results, total, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	// Both have RRF score = 1/(60+1), so order is non-deterministic but both should be present
	ids := map[string]bool{results[0].ID: true, results[1].ID: true}
	assert.True(t, ids["a"])
	assert.True(t, ids["b"])
}

func TestHybridRetriever_LimitAndOffset(t *testing.T) {
	results := make([]query.ScoredResult, 5)
	for i := range results {
		results[i] = query.ScoredResult{ID: fmt.Sprintf("n%d", i), Title: fmt.Sprintf("N%d", i), Score: float64(5 - i)}
	}

	ret := &staticRetriever{results: results, total: 5}
	hybrid := &query.HybridRetriever{
		Retrievers: []query.Retriever{ret},
		K:          60,
	}

	res, total, err := hybrid.Search(context.Background(), "test", 2, 1, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, res, 2)
}

// errorRetriever always returns an error.
type errorRetriever struct{}

func (r *errorRetriever) Search(_ context.Context, _ string, _, _ int, _ index.SearchFilters) ([]query.ScoredResult, int, error) {
	return nil, 0, fmt.Errorf("retriever error")
}

func TestHybridRetriever_ErrorPropagation(t *testing.T) {
	ret := &staticRetriever{
		results: []query.ScoredResult{{ID: "a", Title: "A", Score: 1.0}},
		total:   1,
	}

	hybrid := &query.HybridRetriever{
		Retrievers: []query.Retriever{ret, &errorRetriever{}},
		K:          60,
	}

	_, _, err := hybrid.Search(context.Background(), "test", 10, 0, index.SearchFilters{})
	assert.Error(t, err, "should propagate retriever errors")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run "TestHybridRetriever" ./internal/query/...`
Expected: FAIL — `HybridRetriever` undefined

- [ ] **Step 3: Implement HybridRetriever**

```go
// internal/query/hybrid_retriever.go
package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/index"
	"golang.org/x/sync/errgroup"
)

// HybridRetriever fuses results from N retrievers using Reciprocal Rank Fusion.
type HybridRetriever struct {
	Retrievers []Retriever
	K          int // RRF constant, default 60
}

type rrfEntry struct {
	result ScoredResult
	score  float64
}

// Search runs all retrievers concurrently, then fuses their ranked lists via RRF.
func (h *HybridRetriever) Search(ctx context.Context, query string, limit, offset int, filters index.SearchFilters) ([]ScoredResult, int, error) {
	k := h.K
	if k <= 0 {
		k = 60
	}

	// Fetch a generous number of results from each retriever for good fusion.
	// Each retriever returns up to fetchLimit results; we fuse them all.
	fetchLimit := limit + offset
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	type retrieverResult struct {
		results []ScoredResult
	}

	perRetriever := make([]retrieverResult, len(h.Retrievers))

	g, gCtx := errgroup.WithContext(ctx)
	for i, ret := range h.Retrievers {
		g.Go(func() error {
			results, _, err := ret.Search(gCtx, query, fetchLimit, 0, filters)
			if err != nil {
				return fmt.Errorf("retriever %d: %w", i, err)
			}
			perRetriever[i] = retrieverResult{results: results}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, err
	}

	// Compute RRF scores: for each note, sum 1/(K+rank) across all retrievers
	rrfScores := make(map[string]*rrfEntry)
	for _, rr := range perRetriever {
		for rank, result := range rr.results {
			rrfScore := 1.0 / float64(k+rank+1) // rank is 0-based, RRF uses 1-based
			if entry, ok := rrfScores[result.ID]; ok {
				entry.score += rrfScore
			} else {
				rrfScores[result.ID] = &rrfEntry{
					result: result,
					score:  rrfScore,
				}
			}
		}
	}

	// Sort by RRF score descending
	entries := make([]rrfEntry, 0, len(rrfScores))
	for _, e := range rrfScores {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	total := len(entries)

	// Apply offset/limit
	if offset >= len(entries) {
		return nil, total, nil
	}
	entries = entries[offset:]
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	results := make([]ScoredResult, len(entries))
	for i, e := range entries {
		results[i] = e.result
		results[i].Score = e.score
	}

	return results, total, nil
}
```

Note: This requires `golang.org/x/sync`. Check if it's already a dependency:

```bash
grep "golang.org/x/sync" go.mod
```

If not present, run: `go get golang.org/x/sync`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestHybridRetriever" ./internal/query/...`
Expected: PASS — all 6 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/query/hybrid_retriever.go internal/query/hybrid_retriever_test.go
# Also add go.mod/go.sum if golang.org/x/sync was added
git commit -m "feat(query): add N-way RRF HybridRetriever"
```

---

### Task 7: Config Registry — index.embed and search.mode

**Files:**
- Modify: `internal/config/commands/index_config.go`
- Modify: `internal/config/commands/search_config.go`

- [ ] **Step 1: Add `app.index.embed` config option**

In `internal/config/commands/index_config.go`, add to the `IndexOptions()` return slice:

```go
{
	Key:          "app.index.embed",
	DefaultValue: false,
	Description:  "Compute and store embeddings for note bodies",
	Type:         "bool",
	Required:     false,
	Example:      "true",
},
```

Also add `"app.index.embed": "embed"` to `IndexMetadata.FlagOverrides`.

- [ ] **Step 2: Add `app.search.mode` config option**

In `internal/config/commands/search_config.go`, add to the `SearchOptions()` return slice:

```go
{Key: "app.search.mode", DefaultValue: "keyword", Description: "Search mode: keyword, semantic, or hybrid", Type: "string"},
```

Also add `"app.search.mode": "mode"` to `SearchMetadata.FlagOverrides`.

- [ ] **Step 3: Regenerate config key constants**

Run: `task generate:config:key-constants`

- [ ] **Step 4: Verify the new constants exist**

Run: `grep "KeyAppIndexEmbed\|KeyAppSearchMode" .ckeletin/pkg/config/keys_generated.go`
Expected: Both `KeyAppIndexEmbed` and `KeyAppSearchMode` appear

- [ ] **Step 5: Run task check**

Run: `task check`
Expected: All checks pass

- [ ] **Step 6: Commit**

```bash
git add internal/config/commands/index_config.go internal/config/commands/search_config.go .ckeletin/pkg/config/keys_generated.go
git commit -m "feat(config): add index.embed and search.mode config options"
```

---

### Task 8: Wire --embed Flag into Index Command

**Files:**
- Modify: `cmd/index.go`

- [ ] **Step 1: Write the modified cmd/index.go**

Update `cmd/index.go` to add the `--embed` flag wiring. The command should:
1. Read the `--embed` flag
2. After successful index (rebuild or incremental), if `--embed` is set, create an embedder and call `EmbedNotes`
3. Report embed stats alongside index stats

```go
func runIndex(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppIndexVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppIndexJson)
	fullRebuild := getConfigValueWithFlags[bool](cmd, "full", config.KeyAppIndexFull)
	embed := getConfigValueWithFlags[bool](cmd, "embed", config.KeyAppIndexEmbed)

	info, err := os.Stat(vaultPath)
	if err != nil || !info.IsDir() {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "index", "vault_not_found",
				fmt.Sprintf("vault path %q does not exist or is not a directory", vaultPath))
		}
		return fmt.Errorf("vault path %q does not exist or is not a directory", vaultPath)
	}

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "index", "config_error",
				fmt.Sprintf("loading config: %v", err))
		}
		return fmt.Errorf("loading config: %w", err)
	}

	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	idxr := index.NewIndexer(vaultPath, dbPath, cfg)

	var result *index.IndexResult
	if fullRebuild {
		result, err = idxr.Rebuild()
		if err != nil {
			return fmt.Errorf("rebuilding index: %w", err)
		}
		result.FullRebuild = true
	} else {
		result, err = idxr.Incremental()
		if err != nil {
			return fmt.Errorf("incremental index: %w", err)
		}
	}

	var embedResult *index.EmbedResult
	if embed {
		embedder, embErr := embedding.NewHugotEmbedder(embedding.HugotConfig{
			ModelName:    "sentence-transformers/all-MiniLM-L6-v2",
			CacheDir:     filepath.Join(os.Getenv("HOME"), ".vaultmind", "models"),
			Dims:         384,
			OnnxFilePath: "onnx/model.onnx",
		})
		if embErr != nil {
			return fmt.Errorf("creating embedder: %w", embErr)
		}
		defer func() { _ = embedder.Close() }()

		embedResult, err = idxr.EmbedNotes(cmd.Context(), dbPath, embedder)
		if err != nil {
			return fmt.Errorf("embedding notes: %w", err)
		}
	}

	if jsonOut {
		type indexResponse struct {
			Index *index.IndexResult `json:"index"`
			Embed *index.EmbedResult `json:"embed,omitempty"`
		}
		env := envelope.OK("index", indexResponse{Index: result, Embed: embedResult})
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	if err := formatIndexResult(result, cmd.OutOrStdout()); err != nil {
		return err
	}
	if embedResult != nil {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Embedded %d notes (%d skipped, %d errors)\n",
			embedResult.Embedded, embedResult.Skipped, embedResult.Errors)
		return err
	}
	return nil
}
```

Add `"github.com/peiman/vaultmind/internal/embedding"` to the import block.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Compiles cleanly

- [ ] **Step 3: Run task check**

Run: `task check`
Expected: All checks pass

- [ ] **Step 4: Commit**

```bash
git add cmd/index.go
git commit -m "feat(cmd): wire --embed flag to index command"
```

---

### Task 9: Wire --mode Flag into Search Command

**Files:**
- Modify: `cmd/search.go`

- [ ] **Step 1: Write the modified cmd/search.go**

Update `cmd/search.go` to support `--mode keyword|semantic|hybrid`:

```go
func runSearch(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind search <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppSearchVault)
	mode := getConfigValueWithFlags[string](cmd, "mode", config.KeyAppSearchMode)

	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "search")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	retriever, cleanup, err := buildRetriever(mode, vdb.DB)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	return query.RunSearch(retriever, query.SearchConfig{
		Query:      args[0],
		Limit:      getConfigValueWithFlags[int](cmd, "limit", config.KeyAppSearchLimit),
		Offset:     getConfigValueWithFlags[int](cmd, "offset", config.KeyAppSearchOffset),
		TypeFilter: getConfigValueWithFlags[string](cmd, "type", config.KeyAppSearchType),
		TagFilter:  getConfigValueWithFlags[string](cmd, "tag", config.KeyAppSearchTag),
		JSONOutput: getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSearchJson),
		VaultPath:  vaultPath,
	}, cmd.OutOrStdout())
}

func buildRetriever(mode string, db *index.DB) (query.Retriever, func(), error) {
	switch mode {
	case "keyword", "":
		return &query.FTSRetriever{DB: db}, nil, nil
	case "semantic":
		has, err := index.HasEmbeddings(db)
		if err != nil {
			return nil, nil, fmt.Errorf("checking embeddings: %w", err)
		}
		if !has {
			return nil, nil, fmt.Errorf("no embeddings found — run 'vaultmind index --embed' first")
		}
		embedder, err := newSearchEmbedder()
		if err != nil {
			return nil, nil, err
		}
		return &query.EmbeddingRetriever{DB: db, Embedder: embedder}, func() { _ = embedder.Close() }, nil
	case "hybrid":
		has, err := index.HasEmbeddings(db)
		if err != nil {
			return nil, nil, fmt.Errorf("checking embeddings: %w", err)
		}
		if !has {
			return nil, nil, fmt.Errorf("no embeddings found — run 'vaultmind index --embed' first")
		}
		embedder, err := newSearchEmbedder()
		if err != nil {
			return nil, nil, err
		}
		return &query.HybridRetriever{
			Retrievers: []query.Retriever{
				&query.FTSRetriever{DB: db},
				&query.EmbeddingRetriever{DB: db, Embedder: embedder},
			},
			K: 60,
		}, func() { _ = embedder.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unknown search mode %q (use keyword, semantic, or hybrid)", mode)
	}
}

func newSearchEmbedder() (*embedding.HugotEmbedder, error) {
	embedder, err := embedding.NewHugotEmbedder(embedding.HugotConfig{
		ModelName:    "sentence-transformers/all-MiniLM-L6-v2",
		CacheDir:     filepath.Join(os.Getenv("HOME"), ".vaultmind", "models"),
		Dims:         384,
		OnnxFilePath: "onnx/model.onnx",
	})
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}
	return embedder, nil
}
```

Add these imports:

```go
"os"
"path/filepath"
"github.com/peiman/vaultmind/internal/embedding"
"github.com/peiman/vaultmind/internal/index"
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Compiles cleanly

- [ ] **Step 3: Run task check**

Run: `task check`
Expected: All checks pass

- [ ] **Step 4: Commit**

```bash
git add cmd/search.go
git commit -m "feat(cmd): add --mode flag to search for semantic/hybrid retrieval"
```

---

### Task 10: Wire Hybrid Auto-Detection into Ask Command

**Files:**
- Modify: `cmd/ask.go`

- [ ] **Step 1: Write the modified cmd/ask.go**

Update `cmd/ask.go` to auto-detect hybrid mode when embeddings exist:

```go
func runAsk(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind ask <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppAskVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "ask")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	// Auto-detect: use hybrid if embeddings exist, otherwise keyword
	var retriever query.Retriever
	var cleanup func()
	has, hasErr := index.HasEmbeddings(vdb.DB)
	if hasErr == nil && has {
		retriever, cleanup, err = buildRetriever("hybrid", vdb.DB)
		if err != nil {
			// Fall back to keyword on embedder init failure
			retriever = &query.FTSRetriever{DB: vdb.DB}
		}
	} else {
		retriever = &query.FTSRetriever{DB: vdb.DB}
	}
	if cleanup != nil {
		defer cleanup()
	}

	resolver := graph.NewResolver(vdb.DB)
	result, err := query.Ask(retriever, resolver, vdb.DB, query.AskConfig{
		Query:       args[0],
		Budget:      getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
		MaxItems:    getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
		SearchLimit: getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
	})
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}
	if !getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		return query.FormatAsk(result, cmd.OutOrStdout())
	}
	env := envelope.OK("ask", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = vdb.GetIndexHash()
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}
```

Note: `buildRetriever` is defined in `cmd/search.go` — it's in the same `cmd` package so it's accessible. Add these imports:

```go
"github.com/peiman/vaultmind/internal/index"
"github.com/peiman/vaultmind/internal/query"
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Compiles cleanly

- [ ] **Step 3: Run task check**

Run: `task check`
Expected: All checks pass

- [ ] **Step 4: Commit**

```bash
git add cmd/ask.go
git commit -m "feat(cmd): auto-detect hybrid mode in ask when embeddings exist"
```

---

### Task 11: Final Integration Verification

- [ ] **Step 1: Run full test suite**

Run: `task test`
Expected: All tests pass, coverage ≥85%

- [ ] **Step 2: Run full check**

Run: `task check`
Expected: All checks pass

- [ ] **Step 3: Manual smoke test (if VAULTMIND_TEST_EMBEDDING=1 is available)**

```bash
go build -o /tmp/vaultmind .
/tmp/vaultmind index --vault vaultmind-vault --embed
/tmp/vaultmind search --vault vaultmind-vault --mode keyword "spreading activation"
/tmp/vaultmind search --vault vaultmind-vault --mode semantic "memory consolidation"
/tmp/vaultmind search --vault vaultmind-vault --mode hybrid "knowledge retrieval"
/tmp/vaultmind ask --vault vaultmind-vault "how does spreading activation work"
```

Expected: All commands produce results. Semantic/hybrid modes return results in a different order than keyword.

- [ ] **Step 4: Run task check one final time**

Run: `task check`
Expected: All checks pass

- [ ] **Step 5: Final commit if any formatting/lint fixes were needed**

```bash
git add -A
git commit -m "fix: address lint/format issues from integration"
```
