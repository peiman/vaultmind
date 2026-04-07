---
id: concept-matryoshka-embeddings
type: concept
title: Matryoshka Embeddings
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Matryoshka Representation Learning
  - MRL
  - Nested Embeddings
tags:
  - embedding
  - architecture
  - efficiency
related_ids:
  - concept-open-source-embedding-models
  - concept-embedding-based-retrieval
source_ids: []
---

## Overview

Matryoshka Representation Learning (MRL), introduced by Kusupati et al. (2022), is a training technique that produces embeddings with a special nesting property: the first `d` dimensions of a full-dimensional embedding are themselves a meaningful representation at dimension `d`. Truncating a 768-dim Matryoshka embedding to 256 dims retains most of its retrieval quality — the model is trained to pack the most important information into the leading dimensions.

The name references Russian nesting dolls (матрёшка): each smaller representation is contained within the larger one. MRL training adds a multi-scale loss term: the embedding must be useful at multiple truncation sizes simultaneously (e.g., 768, 512, 256, 128, 64).

**Adoption:** Matryoshka training has been adopted by multiple production and open-source embedding models:
- Nomic-Embed-Text-v1.5 (768 dims, MRL down to 64)
- Snowflake Arctic Embed (multiple sizes)
- Google's text-embedding-004 and EmbeddingGemma
- OpenAI's text-embedding-3 series (256–3072 dims, MRL-enabled)

## Key Properties

- **Flexible storage-quality tradeoff:** A single model can serve multiple precision tiers. Store full 768-dim vectors; search with 256-dim truncations for speed; use full dimensions for final precision pass.
- **Backward compatible retrieval:** Unlike product quantization (which also reduces storage), MRL truncation preserves the original vector space structure. A 256-dim MRL vector and a 768-dim MRL vector are comparable; they live in the same space.
- **Quality retention:** Empirically, MRL models retain ~95% of retrieval nDCG@10 when truncated to 50% of dimensions, and ~85–90% at 25% of dimensions. Below 12.5%, quality degrades significantly.
- **Training overhead:** MRL adds minimal training cost — the multi-scale loss is computed over the same forward pass. Inference cost is identical to standard embedding models.
- **Not the same as PCA:** Principal Component Analysis also reduces dimensionality but requires a separate projection matrix derived from the data distribution. MRL truncation requires no additional computation — just slice the vector.
- **Must be MRL-trained:** Standard embedding models (e.g., BGE-base-en-v1.5) cannot be naively truncated; their first N dimensions are not semantically privileged. Only MRL-trained models support truncation.

## Connections

Matryoshka Embeddings are a property of specific [[open-source-embedding-models|Open-Source Embedding Models]] (notably Nomic-Embed-Text-v1.5 and Snowflake Arctic Embed). The flexible dimension property is directly relevant to [[embedding-based-retrieval|Embedding-Based Retrieval]] in storage-constrained environments.

VaultMind v2: with Matryoshka embeddings, VaultMind could store 768-dim vectors in the SQLite BLOB column but search using 256-dim truncations for speed, falling back to full 768-dim comparison for precision when the initial truncated search returns borderline scores. In practice, for a 123-note vault this tradeoff is unnecessary — at small scale, even 768-dim brute-force cosine is fast. The MRL property becomes valuable as the vault grows past several thousand notes: it allows reducing the embedding BLOB column from ~6KB per note (768 dims × 4 bytes × 2 for float32) to ~2KB (256 dims) without replacing the stored model or re-embedding.
