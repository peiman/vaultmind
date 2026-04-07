# v2 Embedding Support — Design Spec

**Date:** 2026-04-07
**Goal:** Add BGE-M3 embedding-based retrieval via onnxruntime-purego. Ship model with binary.

---

## Architecture

```
index --embed → tokenize → BGE-M3 (ONNX) → embedding BLOB → SQLite
search query → tokenize → BGE-M3 → cosine vs stored vectors → RRF with BM25 → results
```

## Implementation Plan

### Task 1: Add onnxruntime-purego dependency and prove it works

- `go get github.com/shota3506/onnxruntime-purego`
- Write a test that loads a small ONNX model and produces an embedding
- Verify no CGO needed at build time
- Ship `libonnxruntime` alongside binary

### Task 2: Model management — download and cache BGE-M3

- Download BGE-M3 ONNX from HuggingFace on first use
- Cache in `~/.vaultmind/models/bge-m3/`
- Files needed: `model.onnx`, `tokenizer.json`, `config.json`
- `vaultmind model download` command (or auto-download on first `index --embed`)

### Task 3: Embedder interface and BGE-M3 implementation

```go
// internal/embedding/embedder.go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    Dims() int
    Close() error
}

// internal/embedding/bgem3.go
type BGEM3Embedder struct {
    session *ort.Session
    tokenizer *tokenizer.Tokenizer
}
```

### Task 4: Embed during indexing

- Add `--embed` flag to `index` command
- After indexing notes, compute embeddings for each note's body
- Store in `notes.embedding` BLOB column (migration 003 already added)
- Skip notes that already have embeddings (incremental)

### Task 5: EmbeddingRetriever + HybridRetriever

```go
// internal/query/embedding_retriever.go
type EmbeddingRetriever struct {
    DB       *index.DB
    Embedder embedding.Embedder
}

func (r *EmbeddingRetriever) Search(ctx, query, limit, offset, filters) ([]ScoredResult, int, error) {
    // 1. Embed the query
    // 2. Load all embeddings from DB
    // 3. Cosine similarity
    // 4. Sort, limit, return
}

// internal/query/hybrid_retriever.go  
type HybridRetriever struct {
    FTS       *FTSRetriever
    Embedding *EmbeddingRetriever
    K         int // RRF constant, default 60
}
```

## Start with Task 1 — prove the pipeline works
