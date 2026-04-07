---
id: concept-kv-cache
type: concept
title: KV Cache
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Key-Value Cache
  - Attention Cache
  - Prompt Caching
tags:
  - long-context
  - architecture
  - inference
related_ids:
  - concept-infini-attention
  - concept-ring-attention
  - concept-working-memory
source_ids: []
---

## Overview

The KV cache (Key-Value cache) is a standard optimization in transformer-based autoregressive language model inference. During the forward pass, each attention layer computes key and value projections for every token in the sequence. Without caching, generating each new token would require recomputing attention over the entire preceding sequence — O(n) compute per token, or O(n²) for the full generation. The KV cache stores the computed key and value tensors from previous tokens so that each generation step only needs to compute attention for the newly added token against the cached history.

The cache grows linearly with sequence length and is bounded by available GPU memory. For long-context deployments (100K+ token context windows), KV cache memory becomes the dominant constraint — often larger than the model weights themselves for large batches or long sequences.

## Key Properties

- **Memory cost:** Each token requires storing 2 × n_layers × n_heads × d_head floats (one key tensor, one value tensor per layer). For Llama-3 70B at fp16, this is approximately 0.5 MB per token — a 100K context window requires ~50 GB just for the KV cache.
- **PagedAttention (vLLM):** Manages KV cache memory using virtual memory techniques, allocating cache in fixed-size blocks rather than contiguous reservations. Eliminates internal fragmentation and enables efficient memory sharing across requests with common prefixes.
- **Prompt caching (Anthropic, OpenAI):** Provider-side caching of KV tensors for static prefixes (system prompts, document prefixes). If the prefix has been seen recently, the provider can skip its recomputation and charge reduced input token rates. Prefix must be deterministic and above a minimum length threshold to qualify.
- **KV compression:** Techniques such as H2O (Heavy Hitter Oracle), Scissorhands, and SnapKV selectively evict low-importance KV pairs from the cache, reducing memory at the cost of some accuracy. Importance is typically estimated by attention scores.
- **Quantized KV cache:** Storing key/value tensors in lower precision (int8, fp8) to reduce memory footprint with minimal quality degradation. Commonly combined with PagedAttention in production serving systems.
- **Multi-Query Attention (MQA) / Grouped-Query Attention (GQA):** Architectural variants that reduce KV cache size by sharing key/value heads across multiple query heads. GQA (used in Llama-3, Mistral) is the dominant production approach.

## Connections

Prompt caching by LLM providers directly benefits VaultMind's [[context-pack|Context Pack]] workflow. The system prompt plus the assembled context-pack output constitute a stable prefix that can be cached across multiple agent calls within a session. Once the provider caches this prefix, subsequent calls in the same session pay only for the incremental tokens (the new user turn and assistant response), not the full context re-encoding. This makes VaultMind's approach — assembling a rich, dense context prefix once — more efficient than interleaving knowledge retrieval throughout the conversation. The [[working-memory|Working Memory]] analogy applies: just as humans hold a stable context in working memory while reasoning, the cached KV state holds the agent's knowledge context across turns without re-encoding overhead.
