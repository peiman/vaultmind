---
id: decision-bge-m3-implementation
type: decision
status: accepted
title: "BGE-M3 3-in-1 via hugot pure Go backend with Go heads"
created: 2026-04-08
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

Implement BGE-M3's three retrieval modes (dense, sparse, ColBERT) using hugot's pure Go backend (no CGO, no ORT). Bypass hugot's FeatureExtractionPipeline to access raw per-token hidden states, then apply the three heads as pure Go matrix operations. Load sparse_linear.pt and colbert_linear.pt weights via a Go torch loader.

## Performance Verification (2026-04-08)

Tested BGE-M3 (568M params) with hugot pure Go backend on Apple Silicon:
- Embedder init: 10s (one-time model load)
- 2 short texts: 1.9s (~950ms/text)
- 1 longer text (3000 chars): 15s
- Full vault (129 notes): estimated 2-3 minutes

Acceptable for a personal vault. No ORT/CGO needed.

## Context

BGE-M3's ONNX export (`BAAI/bge-m3/onnx/model.onnx`) contains ONLY the base XLMRobertaModel. It outputs a single tensor `last_hidden_state` of shape `[batch, seq_len, 1024]`. The three retrieval heads are separate PyTorch weight files NOT included in the ONNX model:

- `sparse_linear.pt` (3.52 KB): `Linear(1024, 1)` — token-to-scalar weight
- `colbert_linear.pt` (2.1 MB): `Linear(1024, 1024)` — per-token projection

hugot v0.7.0's `FeatureExtractionPipeline` always mean-pools 3D outputs — no raw per-token access. However, ORT backend internal fields are exported, enabling direct session access.

## Head Implementations (Pure Go)

**Dense**: `last_hidden_state[:, 0]` (CLS token) → L2-normalize. No extra weights.

**Sparse**: For each token: `weight = ReLU(dot(hidden, sparse_weights) + bias)`. Scatter weights to vocab positions via input_ids. Zero out special tokens. Output: sparse `map[int32]float32` per input.

**ColBERT**: For each token (skip CLS): `vec = matmul(hidden, colbert_weights) + bias` → L2-normalize per token. Output: `[seq_len-1, 1024]` per input.

## Storage

Three BLOB columns in notes table: `embedding` (dense, existing), `sparse_embedding` (new), `colbert_embedding` (new).

- Dense: raw float32 (1024 × 4 = 4KB/note)
- Sparse: compressed pairs `int32:float32` (~200 entries × 8 = 1.6KB/note)
- ColBERT: raw float32 matrix (seq_len × 1024 × 4, ~2MB/note at max length)

## Alternatives Considered

1. **Custom ONNX export with baked-in heads**: Requires Python in build pipeline. Rejected for simplicity.
2. **Dense-only upgrade, add heads later**: Lower risk but user chose full 3-in-1 in single pass.
3. **onnxruntime-purego (no CGO)**: Marked unstable with no releases. Rejected for reliability.

## Consequences

- No CGO required — pure Go backend handles BGE-M3 (verified)
- Must handle model.onnx_data download (hugot's DownloadModel misses it)
- Must load two PyTorch weight files (sparse_linear.pt, colbert_linear.pt) from model cache via Go torch loader
- Three new Retriever implementations plug into existing N-way RRF HybridRetriever
- Query-time embedding ~1s per query (noticeable but tolerable for CLI)
