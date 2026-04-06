---
id: source-borgeaud-2022
type: source
title: "Borgeaud, S., et al. (2022). Improving language models by retrieving from trillions of tokens. ICML 2022."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2112.04426"
aliases:
  - Borgeaud 2022
  - RETRO paper
tags:
  - llm-memory
  - retrieval-augmented
related_ids:
  - concept-retro
  - concept-rag
  - concept-embedding-based-retrieval
---

# Borgeaud et al. — RETRO (2022)

Borgeaud et al. (DeepMind) introduced RETRO, a 7.5B parameter language model that retrieves from a 2 trillion token database using a frozen BERT retriever. The paper demonstrated that retrieval can substitute for parametric knowledge at a roughly 25:1 ratio — a 7.5B RETRO model matches the perplexity of a 175B GPT-3 model on Pile benchmarks.

The central architectural contribution is **chunked cross-attention**: input sequences are chunked into 64-token segments, each chunk retrieves its nearest neighbors from the database, and generation is conditioned on retrieved chunks via a dedicated cross-attention mechanism. This fine-grained retrieval granularity is more effective than document-level retrieval because it aligns retrieved context tightly with the specific tokens being generated.

The RETRO database is built offline from MassiveText and queried via SCaNN approximate nearest-neighbor search over BERT embeddings. The retriever is frozen — no gradient flows through retrieval — which separates retrieval quality from generation quality as independent engineering concerns.

Key finding with direct relevance to VaultMind: retrieval quality (what is retrieved) matters more than model size above a certain scale. This motivates investment in retrieval architecture over model scaling as a primary path to capability improvement. The chunking insight also suggests that VaultMind's current note-level retrieval granularity may be coarser than optimal.
