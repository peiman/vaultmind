---
id: source-wang-2023-longmem
type: source
title: "Wang, W., et al. (2023). Augmenting Language Models with Long-Term Memory. NeurIPS 2023."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2306.07174"
aliases:
  - Wang 2023 LongMem
  - LongMem paper
tags:
  - llm-memory
  - long-context
related_ids:
  - concept-longmem
  - concept-memgpt
  - concept-memorizing-transformers
---

# Wang et al. — LongMem (NeurIPS 2023)

Wang et al. introduce LongMem, an architecture for augmenting frozen large language models with persistent long-term memory. The core contribution is a decoupled design: the base LLM is frozen and used purely as a memory encoder, while a trainable adaptive residual side-network handles retrieval and integration of cached past contexts. Memory is stored in a memory bank holding up to 65,536 tokens of prior context, encoded once and reused without re-encoding, avoiding the staleness that would arise if the backbone were updated.

At inference time, the side-network retrieves relevant memory entries from the bank and injects them into the LLM's forward pass via residual connections. This allows the model to attend to far more context than its native window permits, without any modification to the backbone's weights. The side-network is lightweight and trained to complement the frozen backbone, learning which cached contexts are worth incorporating for a given input.

Evaluation on ChapterBreak — a benchmark requiring coherent reasoning across long narrative spans — shows LongMem outperforming strong long-context baselines including models with extended context windows.

## Key Findings

- Decoupled frozen-backbone + side-network design eliminates memory staleness; cached contexts encoded once remain valid indefinitely
- 65K-token memory bank provides effective context far beyond standard transformer context limits, without quadratic attention costs over full context
- Side-network residual injection is parameter-efficient: only the small adapter is trained, not the full backbone
- ChapterBreak: LongMem outperforms baselines including vanilla long-context models, demonstrating that cached-context retrieval is competitive with extended native context windows
- The plug-in nature means LongMem can be applied to arbitrary frozen LLMs, making it practical for deployment on top of already-trained models

## Relevance to VaultMind

LongMem directly motivates VaultMind's role as a persistent encoded memory store. The vault is analogous to LongMem's memory bank: a collection of past contexts (notes) encoded at write time and available for retrieval at query time. A future VaultMind integration could expose vault embeddings to a LongMem-style side-network, enabling an agent to learn retrieval policies over vault notes rather than relying on static similarity metrics. The frozen-backbone insight also justifies VaultMind's design choice to treat vault contents as stable reference data rather than dynamic model state.
