---
id: concept-bge-m3-implementation
type: concept
title: "BGE-M3 Implementation Architecture"
status: active
created: 2026-04-08
tags:
  - embedding
  - bge-m3
  - implementation
related_ids:
  - concept-open-source-embedding-models
  - concept-hybrid-search
  - concept-colbert
  - concept-onnx-inference
---

## Overview

BGE-M3 (BAAI, 568M params, MIT license) produces three retrieval signals from a single forward pass: dense embeddings, learned sparse weights, and ColBERT per-token embeddings. However, the standard ONNX export contains ONLY the base XLMRobertaModel transformer — the three retrieval heads are stored as separate PyTorch weight files, not baked into the ONNX model.

## ONNX Model Structure

The ONNX model at `BAAI/bge-m3/onnx/model.onnx` outputs a single tensor:
- **`last_hidden_state`**: shape `[batch, seq_len, 1024]` — per-token hidden states from the final transformer layer

The three retrieval heads are NOT in the ONNX model:
- **`sparse_linear.pt`** (3.52 KB): `Linear(1024, 1)` — projects each token to a scalar weight
- **`colbert_linear.pt`** (2.1 MB): `Linear(1024, 1024)` — projects each token for late interaction

## Head Implementations (Must Be Done in Go)

### Dense Embedding
- Take `last_hidden_state[:, 0]` (CLS token, not mean pooling)
- L2-normalize
- Output: `[batch, 1024]`
- No extra weights needed

### Sparse Embedding (Learned Lexical Weights)
- Load `sparse_linear.pt` weights (1024 floats + 1 bias)
- For each token: `weight = ReLU(sparse_linear(hidden_state))`
- Scatter weights to vocabulary positions using input_ids
- Zero out special tokens (CLS, EOS, PAD, UNK)
- Output: sparse vector `[vocab_size=250002]` per input
- Used for BM25-like retrieval with learned (not hand-crafted) term weights

### ColBERT Embedding (Per-Token Late Interaction)
- Load `colbert_linear.pt` weights (1024×1024 matrix)
- For each token (skip CLS at index 0): `vec = colbert_linear(hidden_state)`
- L2-normalize each token vector
- Output: `[batch, seq_len-1, 1024]`
- Used for MaxSim scoring: for each query token, find max cosine with any doc token, sum across query tokens

## hugot v0.7.0 Limitations

- `FeatureExtractionPipeline` always mean-pools 3D outputs — no raw per-token access
- Only selects ONE output tensor per pipeline (via `OutputIndex` or `WithOutputName`)
- ORT backend captures all outputs but pipeline discards extras in postprocessing
- Internal fields are exported (`Model.ORTModel.Session`, `OutputsMeta`) enabling direct access

## Implementation Paths

1. **Bypass hugot pipeline for inference** — use hugot for model download/tokenization, access ORT session directly for inference with full output control
2. **Custom ONNX model** — export from Python with all three heads baked in as separate named outputs
3. **Implement heads in Go** — load sparse_linear.pt and colbert_linear.pt weights, apply to last_hidden_state with simple matrix multiplication

Path 1+3 is the pragmatic choice: use hugot's infrastructure but bypass its pipeline postprocessing to get raw token outputs, then implement the three heads as pure Go matrix ops.

## Key Differences from all-MiniLM-L6-v2

| Property | all-MiniLM-L6-v2 | BGE-M3 |
|----------|-------------------|--------|
| Params | 22M | 568M |
| Dims | 384 | 1024 |
| Context | 512 tokens | 8192 tokens |
| MTEB Retrieval | ~49 | ~63 |
| Pooling | Mean pooling | CLS pooling (dense) |
| Outputs | Dense only | Dense + Sparse + ColBERT |
| Runtime | hugot pure-Go | hugot ORT backend (CGO) |
| License | Apache 2.0 | MIT |
