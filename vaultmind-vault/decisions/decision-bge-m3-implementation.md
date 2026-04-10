---
id: decision-bge-m3-implementation
type: decision
status: accepted
title: "BGE-M3 3-in-1 via ORT backend with Go heads"
created: 2026-04-08
vm_updated: 2026-04-09
tags:
  - architecture
  - embedding
  - bge-m3
related_ids:
  - concept-open-source-embedding-models
  - concept-hybrid-search
  - concept-colbert
  - concept-onnx-inference
  - decision-structured-over-embeddings
---

## Decision

Implement BGE-M3's three retrieval modes (dense, sparse, ColBERT) using hugot with ONNX Runtime backend for indexing. Bypass hugot's FeatureExtractionPipeline to access raw per-token hidden states, then apply the three heads as pure Go matrix operations. Build with `-tags ORT` for BGE-M3 support; default build uses pure Go backend for MiniLM.

## Performance (Verified 2026-04-09, Apple Silicon M4 Max)

| Backend | Index (130 notes) | Query | Status |
|---------|-------------------|-------|--------|
| ORT CPU | **23 min** | **6s** | Shipping — works |
| Pure Go | Hours (unusable) | ~1s short text | Too slow for indexing |
| ORT + CoreML | Failed | — | XLMRoberta ops unsupported by CoreML EP |
| coremltools conversion | Failed | — | int cast op unsupported in converter |

Pure Go backend bench test was misleading: tested with 10-token texts (~1s), but real notes average 800 tokens. Transformer attention is O(n2) — scaling is quadratic, not linear.

## Context

BGE-M3's ONNX export (`BAAI/bge-m3/onnx/model.onnx`) contains ONLY the base XLMRobertaModel. It outputs a single tensor `last_hidden_state` of shape `[batch, seq_len, 1024]`. The three retrieval heads are separate weight files NOT included in the ONNX model:

- `sparse_linear.pt` (3.52 KB): `Linear(1024, 1)` — token-to-scalar weight
- `colbert_linear.pt` (2.1 MB): `Linear(1024, 1024)` — per-token projection

## Head Implementations (Pure Go)

**Dense**: `last_hidden_state[:, 0]` (CLS token, not mean pooling) then L2-normalize. No extra weights.

**Sparse**: For each non-special token: `weight = ReLU(dot(hidden, sparse_weights) + bias)`. Scatter to vocab positions via input_ids. Duplicate tokens keep max weight. Output: sparse `map[int32]float32`.

**ColBERT**: For each non-CLS token: `vec = matmul(hidden, colbert_weights) + bias` then L2-normalize. Output: `[seq_len-1, 1024]`.

## Storage

Three BLOB columns in notes table: `embedding` (dense), `sparse_embedding` (new), `colbert_embedding` (new, with 4-byte dims header).

- Dense: raw float32 (1024 x 4 = 4KB/note)
- Sparse: packed int32:float32 pairs (~200 entries x 8 = 1.6KB/note)
- ColBERT: 4-byte dims header + raw float32 matrix (variable, ~200KB-2MB/note)

## GPU Investigation (2026-04-09)

Three GPU paths attempted, all failed for BGE-M3:

1. **ORT CoreML Execution Provider**: Cannot handle external data files (model.onnx + model.onnx_data split). Even after restructuring the data layout, fails with "unknown exception in Initialize()" — XLMRoberta's dynamic ops are unsupported.

2. **coremltools PyTorch-to-CoreML conversion**: torch.jit.trace succeeds but coremltools converter fails on `int` cast op in XLMRoberta's attention mechanism ("only 0-dimensional arrays can be converted to Python scalars").

3. **ONNX model merge** (eliminate external data): Protobuf has 2GB serialization limit. Model is 2.2GB so cannot be a single file.

Conclusion: GPU acceleration for BGE-M3 on Apple Silicon is blocked by Apple's CoreML tooling, not by VaultMind. Future coremltools releases may add XLMRoberta support.

## Build Configuration

- `go build` (default): Pure Go backend. MiniLM works fine. BGE-M3 too slow for indexing.
- `go build -tags ORT`: ONNX Runtime backend. Required for BGE-M3 indexing. Needs `libonnxruntime` + `libtokenizers` installed.
- Build-tag-conditional session factory: `session_go.go` vs `session_ort.go` with `//go:build` tags.
- Auto-detect `libonnxruntime` location: checks `ORT_LIB_DIR` env, `/opt/homebrew/lib`, `/usr/local/lib`.

## Alternatives Considered

1. **Pure Go backend for everything**: Verified too slow for BGE-M3 indexing (hours vs 23 min with ORT).
2. **CoreML/GPU acceleration**: Blocked by Apple tooling limitations with XLMRoberta architecture.
3. **onnxruntime-purego (no CGO)**: Marked unstable with no releases. Rejected for reliability.
4. **Dense-only upgrade**: Lower risk but user chose full 3-in-1.

## Consequences

- CGO required at build time for BGE-M3 (`-tags ORT` build tag)
- Must install `libonnxruntime` (via homebrew) and `libtokenizers` (pre-built binary from GitHub)
- Default build (no tags) still works for MiniLM — no regression
- Must handle `model.onnx_data` download (hugot's DownloadModel misses it)
- Weight files loaded via gopickle (handles HalfStorage from PyTorch)
- 4-way RRF hybrid: FTS + Dense + Sparse + ColBERT
- 23 min index time, 6s query time (CPU-only ORT)
