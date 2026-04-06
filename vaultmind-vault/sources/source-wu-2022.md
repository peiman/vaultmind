---
id: source-wu-2022
type: source
title: "Wu, Y., Rabe, M. N., Hutchins, D., & Szegedy, C. (2022). Memorizing Transformers. ICLR 2022."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2203.08913"
aliases:
  - Wu 2022
  - Memorizing Transformers paper
tags:
  - llm-memory
  - external-memory
related_ids:
  - concept-memorizing-transformers
  - concept-retro
---

# Wu et al. — Memorizing Transformers (2022)

Wu et al. (Google Brain) introduced a Transformer variant that augments standard local attention with a non-differentiable external memory of (key, value) pairs accumulated from past input tokens. At each memory-augmented layer, the model performs approximate kNN lookup over stored KV pairs (up to 262,144 entries) and attends over both local context and retrieved memory.

The paper evaluated the architecture on books (PG-19), code (GitHub), and mathematics (arXiv), finding consistent perplexity improvements across all three domains as memory size scaled from 8K to 262K tokens. Crucially, these gains could not be replicated by simply extending the local attention window to equivalent length — the approximate retrieval mechanism provides something structurally different from full attention over a longer context.

Retrieval uses ScaNN (Google's approximate nearest-neighbor library), trading a small accuracy loss for the practical efficiency required to query 262K entries at inference time. The KV pairs are derived from the model's own attention computations on past tokens, not from an external corpus — making memory content input-dependent rather than fixed.

The paper was accepted at ICLR 2022, making it one of the first works to demonstrate that non-differentiable external memory can be integrated into Transformers without degrading training stability. Its architecture influenced subsequent work on long-context modeling and is a precursor to approaches that use external vector stores as model memory.
