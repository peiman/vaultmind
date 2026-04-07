---
id: concept-longmem
type: concept
title: LongMem
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Long-Term Memory Framework
  - Augmenting LMs with Long-Term Memory
tags:
  - llm-memory
  - architecture
  - long-context
related_ids:
  - concept-memgpt
  - concept-memorizing-transformers
  - concept-retro
  - concept-infini-attention
source_ids:
  - source-wang-2023-longmem
---

## Overview

LongMem (Wang et al., NeurIPS 2023) proposes a decoupled architecture for giving language models access to long-term memory without modifying the base LLM. The key insight is that updating a large frozen backbone every time new context arrives is computationally prohibitive and risks catastrophic forgetting. LongMem solves this by separating concerns: the backbone LLM acts as a frozen memory encoder, while a lightweight adaptive residual side-network handles memory retrieval and reading.

Past contexts are cached in a memory bank — up to 65,536 tokens — and retrieved at inference time by the side-network, which has been trained to incorporate retrieved memories into the LLM's forward pass via residual connections. The frozen backbone never changes; only the small side-network is adapted. This decoupling eliminates memory staleness: old cached contexts are encoded once and remain valid regardless of how many new contexts are processed.

LongMem outperforms strong long-context baselines (including models with longer native context windows) on ChapterBreak, a benchmark requiring coherent understanding across book chapters. arXiv:2306.07174.

## Key Properties

- **Decoupled architecture:** Frozen backbone LLM (memory encoder) + adaptive residual side-network (memory retriever/reader). The backbone requires no gradient updates after initial training.
- **65K token cache:** Memory bank stores past encoded contexts up to 65,536 tokens, far beyond what a standard context window holds at once.
- **No memory staleness:** Because the backbone is frozen, cached encodings remain accurate over time — no need to re-encode old memories when the model is updated.
- **Residual integration:** Retrieved memories are injected into the LLM's forward pass via residual connections in the side-network, allowing memory to influence generation without architectural surgery on the base model.
- **ChapterBreak performance:** Outperforms standard long-context models on ChapterBreak, demonstrating the benefit of persistent cache over extended-context attention alone.
- **Plug-in design:** The side-network can in principle be trained for any frozen backbone, making LongMem applicable to off-the-shelf models.

## Connections

LongMem's design is philosophically related to [[infini-attention|Infini-Attention]] and [[memorizing-transformers|Memorizing Transformers]] — all three tackle the problem of extending an LLM's effective memory beyond its context window. LongMem's distinguishing feature is the hard separation between the frozen encoder and the trainable retriever, whereas Infini-Attention integrates compressive memory within attention heads.

The relationship to [[retro|RETRO]] is structural: both use a retrieval mechanism to inject relevant past information into generation. LongMem differs in that it caches the model's own prior contexts rather than a static external corpus.

For VaultMind, LongMem's decoupled memory architecture (frozen encoder + adaptive reader) suggests VaultMind could serve as the "frozen memory" that a lightweight adapter reads from. A VaultMind vault is precisely such a cache of past contexts — encoded notes that persist across sessions. An agent equipped with a LongMem-style side-network could learn to selectively retrieve and inject vault notes into its forward pass, going beyond keyword or embedding search toward learned retrieval policies.
