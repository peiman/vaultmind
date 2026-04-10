# BGE-M3 3-in-1 Upgrade — Design Spec

**Date:** 2026-04-08
**Goal:** Replace all-MiniLM-L6-v2 with BGE-M3 for dense+sparse+ColBERT retrieval using pure Go backend (no CGO/ORT).

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Runtime | hugot pure Go backend | Verified: 568M params work, ~1s/text, no CGO needed |
| 3-in-1 approach | Bypass hugot pipeline, implement heads in Go | ONNX export only has base transformer; heads are separate .pt files |
| Sparse storage | BLOB with packed int32:float32 pairs | Compact (~1.6KB/note), lossless, no schema complexity |
| ColBERT storage | BLOB with raw float32 matrix | Variable length, lossless, consistent with dense pattern |
| PyTorch weight loading | Go torch loader (nlpodyssey/gopickle) | No Python dependency, weights are simple linear layers |
| Model download | Custom download handling model.onnx_data | hugot's DownloadModel misses the external data file |
| Build tags | None — pure Go, standard `go build` | No CGO, no ORT, no libonnxruntime to ship |

## Performance (Verified 2026-04-08)

Tested BGE-M3 with hugot pure Go backend on Apple Silicon:
- Embedder init: 10s (one-time model load)
- Short text (~10 words): ~950ms
- Long text (~3000 chars): ~15s
- Full vault (129 notes): estimated 2-3 minutes
- Query-time embedding: ~1s per query

## Architecture

```
index --embed --model bge-m3:
  notes -> body_text -> TruncateForEmbedding(8190 tokens)
       -> hugot forward pass -> last_hidden_state [batch, seq, 1024]
       -> Dense head:   CLS token [0] -> L2-norm -> notes.embedding BLOB
       -> Sparse head:  sparse_linear(tokens) -> ReLU -> scatter to vocab -> notes.sparse_embedding BLOB
       -> ColBERT head: colbert_linear(tokens[1:]) -> L2-norm -> notes.colbert_embedding BLOB

search --mode hybrid (with BGE-M3 embeddings):
  query -> BGEM3Embedder.EmbedFull() -> dense + sparse + colbert vectors
       -> FTSRetriever      (existing FTS5)          -+
       -> DenseRetriever     (cosine vs embedding)     +-> HybridRetriever (4-way RRF, K=60) -> results
       -> SparseRetriever    (dot product vs sparse)   |
       -> ColBERTRetriever   (MaxSim vs colbert)      -+
```

## Component 1: BGEM3Embedder (`internal/embedding/bgem3.go`)

```go
type BGEM3Embedder struct {
    session   *hugot.Session
    pipeline  *pipelines.FeatureExtractionPipeline
    sparseW   []float32     // [1024] from sparse_linear.pt
    sparseB   float32       // bias from sparse_linear.pt
    colbertW  [][]float32   // [1024][1024] from colbert_linear.pt
    colbertB  []float32     // [1024] bias from colbert_linear.pt
    dims      int           // 1024
    maxTokens int           // 8190
}

type BGEM3Output struct {
    Dense   []float32         // [1024] CLS-pooled, L2-normalized
    Sparse  map[int32]float32 // vocab_id -> weight (non-zero entries only)
    ColBERT [][]float32       // [seq_len-1][1024] per-token, L2-normalized
}
```

**Implements `Embedder` interface** — `Embed()` returns dense vector for backward compatibility.

**New method** — `EmbedFull(ctx, text) (*BGEM3Output, error)` returns all three vectors.

**New method** — `EmbedFullBatch(ctx, texts) ([]*BGEM3Output, error)` for batch index-time embedding.

**Raw output access:** Bypass hugot's `FeatureExtractionPipeline.Postprocess()` to get `last_hidden_state` before mean-pooling. Use hugot's exported internal fields (`pipeline.Model`) or run the ONNX session directly.

### Head implementations (pure Go)

**Dense:**
```
dense = last_hidden_state[0]        // CLS token at position 0
dense = L2Normalize(dense)          // unit vector
```

**Sparse:**
```
for each token i:
    weight = ReLU(dot(hidden[i], sparseW) + sparseB)
    if weight > 0 and token not in {CLS, EOS, PAD, UNK}:
        sparse[input_ids[i]] = max(sparse[input_ids[i]], weight)
```

**ColBERT:**
```
for each token i starting from 1 (skip CLS):
    vec = matmul(hidden[i], colbertW) + colbertB
    colbert[i-1] = L2Normalize(vec)
```

### Model download

Custom `DownloadBGEM3(cacheDir string) (string, error)` that fetches from HuggingFace:
- `onnx/model.onnx` (725KB stub)
- `onnx/model.onnx_data` (~2.1GB weights)
- `tokenizer.json`, `tokenizer_config.json`, `special_tokens_map.json`
- `config.json`
- `sparse_linear.pt` (3.52KB)
- `colbert_linear.pt` (2.1MB)

Stored in `~/.vaultmind/models/BAAI_bge-m3/`. Download progress reported to stderr.

### PyTorch weight loading

Use `nlpodyssey/gopickle` to read `.pt` files in pure Go. Each file contains a single `Linear` layer with `weight` and `bias` tensors. Extract the raw float32 arrays.

Note: PyTorch weight files use Python's serialization format internally. The `gopickle` library handles this safely in Go without executing arbitrary code — it only parses the tensor data structures. The weight files are from a trusted source (BAAI's official HuggingFace repository) and contain only numeric tensors (no executable content).

If gopickle proves problematic, fallback: manually parse the zip+tensor format or pre-extract to raw binary at download time.

## Component 2: Schema Migration (`internal/index/migrations/005_add_sparse_colbert_columns.sql`)

```sql
ALTER TABLE notes ADD COLUMN sparse_embedding BLOB;
ALTER TABLE notes ADD COLUMN colbert_embedding BLOB;
```

## Component 3: Embedding Storage (`internal/index/embedding.go`)

### Sparse encoding

Packed pairs of `(int32 token_id, float32 weight)`:
```
[4 bytes: token_id_1][4 bytes: weight_1][4 bytes: token_id_2][4 bytes: weight_2]...
```

8 bytes per non-zero entry. Typical note: ~50-200 entries = 400-1600 bytes.

```go
func EncodeSparseEmbedding(sparse map[int32]float32) []byte
func DecodeSparseEmbedding(data []byte) (map[int32]float32, error)
func StoreSparseEmbedding(d *DB, noteID string, sparse map[int32]float32) error
func LoadAllSparseEmbeddings(d *DB) ([]NoteSparseEmbedding, error)
```

### ColBERT encoding

Raw float32 matrix, same as dense but variable length:
```
[4 bytes: float_1][4 bytes: float_2]... (seq_len * 1024 floats)
```

Decode infers seq_len from `len(data) / 4 / 1024`.

```go
func EncodeColBERTEmbedding(colbert [][]float32) []byte
func DecodeColBERTEmbedding(data []byte, dims int) ([][]float32, error)
func StoreColBERTEmbedding(d *DB, noteID string, colbert [][]float32) error
func LoadAllColBERTEmbeddings(d *DB) ([]NoteColBERTEmbedding, error)
```

### Extended types

```go
type NoteSparseEmbedding struct {
    NoteID string
    Sparse map[int32]float32
    Type, Title, Path string
    IsDomain bool
}

type NoteColBERTEmbedding struct {
    NoteID  string
    ColBERT [][]float32
    Type, Title, Path string
    IsDomain bool
}
```

## Component 4: New Retrievers (`internal/query/`)

### SparseRetriever (`sparse_retriever.go`)

```go
type SparseRetriever struct {
    DB       *index.DB
    Embedder *embedding.BGEM3Embedder
}
```

**Scoring:** Dot product between query sparse vector and document sparse vector. Only overlapping vocab_ids contribute:
```
score = sum(query_sparse[id] * doc_sparse[id]) for id in intersection(query_keys, doc_keys)
```

Fast — sparse vectors have ~50-200 non-zero entries.

### ColBERTRetriever (`colbert_retriever.go`)

```go
type ColBERTRetriever struct {
    DB       *index.DB
    Embedder *embedding.BGEM3Embedder
}
```

**Scoring (MaxSim):** For each query token vector, find the maximum cosine similarity across all document token vectors, then sum:
```
score = sum over q_tokens: max over d_tokens: cosine(q_vec, d_vec)
```

Most expensive retriever but most precise. At 129 notes with typical seq_len ~200, this is still subsecond.

### BuildRetriever update (`retriever_builder.go`)

`BuildRetriever("hybrid", db)` detects which embedding columns are populated:
- Only `embedding` (dense): FTS + Dense (2-way, current behavior)
- All three columns: FTS + Dense + Sparse + ColBERT (4-way)

Detection via:
```go
func HasSparseEmbeddings(d *DB) (bool, error)
func HasColBERTEmbeddings(d *DB) (bool, error)
```

## Component 5: Config + Command Changes

### Config

New option `app.index.model` (string, default `"minilm"`):
- `"minilm"` — all-MiniLM-L6-v2, 384 dims, dense only
- `"bge-m3"` — BGE-M3, 1024 dims, dense + sparse + ColBERT

### Index command

`vaultmind index --embed --model bge-m3`:
- Downloads BGE-M3 if not cached
- Creates BGEM3Embedder
- Calls `EmbedFull` for each note
- Stores dense, sparse, ColBERT in all three columns

`vaultmind index --embed` (default model):
- Uses MiniLM as today
- Only stores dense embedding

### Migration path

Users upgrading: `vaultmind index --embed --model bge-m3 --full`
- Full rebuild reindexes all notes
- EmbedNotes overwrites all three embedding columns
- Old MiniLM 384-dim vectors replaced with BGE-M3 1024-dim ones

### Search + Ask

No user-facing changes needed. `BuildRetriever` auto-detects available columns and wires the appropriate retrievers into HybridRetriever.

## Testing Strategy

- **BGEM3Embedder**: Gated behind `VAULTMIND_TEST_BGEM3=1` (requires ~2.2GB model download). Test dense/sparse/ColBERT output shapes and basic properties.
- **Encode/Decode**: Pure function tests for sparse and ColBERT serialization (round-trip, edge cases). No model needed.
- **SparseRetriever**: Mock sparse vectors in temp DB, verify dot-product scoring and ranking.
- **ColBERTRetriever**: Mock ColBERT matrices in temp DB, verify MaxSim scoring and ranking.
- **4-way HybridRetriever**: Use existing HybridRetriever tests with 4 static retrievers.
- **Integration**: Index test vault with BGE-M3, verify all three columns populated, search returns results.
- **BuildRetriever auto-detection**: Test that hybrid mode wires 2-way vs 4-way based on populated columns.
