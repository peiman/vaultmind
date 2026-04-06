---
id: source-lewis-2020
type: source
title: "Lewis et al. Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks (2020)"
created: 2026-04-03
vm_updated: 2026-04-03
url: "https://arxiv.org/abs/2005.11401"
aliases:
  - Lewis 2020
  - RAG paper
tags:
  - ai-memory
  - retrieval
related_ids:
  - concept-rag
---

# Lewis et al. — RAG (2020)

Lewis et al. combined a dense retrieval component (DPR) with a seq2seq generator (BART), creating a system that fetches relevant documents from a non-parametric index before generating an answer. The retrieval step grounds generation in external facts, reducing hallucination and enabling the knowledge base to be updated without retraining the language model. Two variants were tested: RAG-Sequence (one retrieval per full output) and RAG-Token (retrieval can vary per output token).

The paper established that separating knowledge storage from reasoning is both practical and beneficial. Parametric memory (model weights) and non-parametric memory (retrieval index) are complementary: the model handles syntax and inference while the index handles facts. This separation is now a cornerstone of applied LLM architecture and is central to [[rag|RAG]] as a vault concept.

VaultMind is architecturally a RAG system where the Obsidian vault is the non-parametric store. Each user query triggers a retrieval pass over embedded note chunks, and the retrieved passages are injected into the generation context. The Lewis et al. paper justifies this design choice and informs the embedding and chunking strategies VaultMind uses to keep retrieval precision high across large vaults.
