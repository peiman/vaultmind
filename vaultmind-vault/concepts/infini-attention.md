---
id: concept-infini-attention
type: concept
title: Infini-Attention
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Infinite Attention
  - Leave No Context Behind
tags:
  - long-context
  - architecture
  - attention
related_ids:
  - concept-working-memory
  - concept-memorizing-transformers
  - concept-lost-in-the-middle
source_ids:
  - source-munkhdalai-2024
---

## Overview

Infini-attention (Munkhdalai, Faruqui, & Gopal, Google, 2024) proposes a drop-in replacement for the standard attention layer that combines compressive long-term memory with vanilla local attention inside a single Transformer block. The key insight is that linear attention — specifically a linear form with associative memory — can store a compressed representation of all past context in a fixed-size memory matrix, while standard dot-product attention handles fine-grained retrieval over a local segment. Both operate within the same layer, and their outputs are mixed via a learned scalar gating parameter.

The paper demonstrated that 1B and 8B parameter models equipped with Infini-attention can process and summarize entire books (500K+ tokens) on BookSum, achieving state of the art at both scales. A separate 1 million token passkey retrieval experiment confirmed that the architecture retains access to information arbitrarily far back in the input sequence.

Published as arXiv:2404.07143 in April 2024.

## Key Properties

- **Bounded memory and compute:** Total memory footprint does not grow with sequence length. The compressive memory matrix has a fixed size regardless of how many tokens have been processed.
- **Two-path attention in one layer:** Masked local attention (causal, standard dot-product over a current segment) and long-term linear attention (associative memory over all prior segments) run in parallel within each Transformer layer.
- **Associative memory update:** Past KV pairs are not stored explicitly — they are projected into a fixed-size matrix via an outer-product update rule. This is lossy but enables infinite context with O(1) memory.
- **Learned gating:** A scalar gate β, learned per layer, controls the balance between local attention output and long-term memory output. Different layers can specialize — some rely on local context, others on compressed memory.
- **No architectural surgery:** Infini-attention replaces standard attention without changing model depth, width, or the rest of the Transformer stack. Existing pre-trained weights can be fine-tuned to use it.
- **SOTA on BookSum (1B and 8B):** Outperformed prior approaches on long-document summarization without increasing model size.

## Connections

Infini-attention is closely related to [[memorizing-transformers|Memorizing Transformers]], which also augment local attention with access to past tokens. The key difference is storage strategy: Memorizing Transformers maintain a non-differentiable KV cache (exact token representations retrieved via kNN), while Infini-attention uses a differentiable compressive memory (lossy matrix that summarizes all past KVs). Memorizing Transformers have higher fidelity at the cost of growing memory; Infini-attention has bounded memory at the cost of compression loss.

The compressive memory mechanism shares structure with work on fast weights and linear recurrent models, bridging Transformers and RNNs.

For VaultMind, Infini-attention illustrates that compressive memory — lossy summarization of past context — can substitute for explicit retrieval in many cases. [[context-pack|Context Pack]] is explicit retrieval (select and inject the right notes); Infini-attention suggests a complementary design where a fine-tuned agent model itself maintains compressed context across very long sessions, reducing dependence on discrete retrieval decisions. As model architectures adopt Infini-attention, the role of VaultMind-style systems shifts toward curation and persistence rather than compensating for short context windows.
