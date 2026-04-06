---
id: source-guu-2020
type: source
title: "Guu, K., Lee, K., Tung, Z., Pasupat, P., & Chang, M.-W. (2020). REALM: Retrieval-Augmented Language Model Pre-Training. ICML 2020."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2002.08909"
aliases:
  - Guu 2020
  - REALM paper
tags:
  - retrieval
  - pre-training
related_ids:
  - concept-realm
  - concept-rag
---

# Guu et al. — REALM (ICML 2020)

Guu et al. introduced the first retrieval-augmented language model trained end-to-end during pre-training. The system frames masked language modeling as a latent-variable problem: for each masked token, the model marginalizes over a distribution of retrieved documents, computing the probability of the correct token under each document and weighting by document relevance. Gradients flow back through both the language model and the retriever, training both simultaneously without labeled retrieval supervision.

The retriever is a dual-encoder (BERT-based) that scores documents by inner product between the query and document representations, identical in structure to [[dense-passage-retrieval|Dense Passage Retrieval]]. To make end-to-end training tractable, document encoder parameters are updated asynchronously: the MIPS index is rebuilt from a snapshot of the encoder every few hundred steps rather than continuously. Salient-span masking (masking named entities and dates preferentially) steers pre-training toward retrieving factual documents.

At ICML 2020, REALM outperformed all prior methods on Open-QA benchmarks by 4–16 percentage points and set a new state of the art on Natural Questions Open. Crucially, the gains held even when the number of parameters matched non-retrieval baselines, demonstrating that retrieval adds value beyond simply increasing model capacity.

## Key Findings

- 4–16% improvement over previous state of the art on Open-QA benchmarks (Natural Questions, WebQuestions, CuratedTREC)
- End-to-end pre-training with retrieval outperforms retrieval added only at fine-tuning time, showing that early integration of retrieval matters
- Salient-span masking is necessary: random masking does not provide a strong enough signal to learn useful document retrieval
- Asynchronous index refresh (every ~500 steps) is sufficient — continuous re-indexing is not required for stable training
- First demonstration that a retriever can be learned without any labeled (query, relevant document) pairs

## Relevance to VaultMind

REALM's core insight — that a retrieval corpus can be integrated into learning from the ground up, not just bolted on at inference — is relevant to any future VaultMind agent fine-tuning strategy. If a VaultMind-specialized LLM is ever trained, REALM's approach of using the vault corpus during pre-training (rather than only at serving time) would likely produce an agent with stronger intrinsic retrieval intuitions. The asynchronous index refresh technique is also directly applicable to keeping VaultMind's own note index current without re-embedding the entire vault on every write.
