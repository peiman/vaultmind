---
id: source-packer-2023
type: source
title: "Packer et al. MemGPT: Towards LLMs as Operating Systems (2023)"
created: 2026-04-03
vm_updated: 2026-04-03
url: "https://arxiv.org/abs/2310.08560"
aliases:
  - Packer 2023
  - MemGPT paper
tags:
  - ai-memory
  - agent-architecture
related_ids:
  - concept-memgpt
  - concept-working-memory
---

# Packer et al. — MemGPT (2023)

Packer et al. proposed treating the LLM context window as a CPU register and external storage as disk, borrowing OS virtual-memory techniques to give language models effectively unbounded memory. The system pages content in and out of the fixed context window on demand, using a hierarchical storage structure: main context (in-window), external context (vector store), and archival storage (cold store). Function calls allow the model itself to trigger memory operations.

The analogy to operating-system paging is significant: it reframes the context-window limitation not as a hard ceiling but as a cache miss problem. This shifts design thinking toward cache-hit rates, eviction policies, and prefetching—all tractable engineering problems. The connection to [[working-memory|Working Memory]] is explicit: main context is working memory, and the paging mechanism is the attentional gateway that decides what stays resident.

VaultMind inherits the tiered-storage framing from [[memgpt|MemGPT]]. Vault notes in active editing sessions occupy the equivalent of main context; the full vault index is external context; archived or rarely accessed notes are cold storage. VaultMind's retrieval pipeline mirrors MemGPT's demand-paging by only loading note content when a relevance threshold is crossed.
