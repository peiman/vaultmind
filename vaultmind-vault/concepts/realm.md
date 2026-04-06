---
id: concept-realm
type: concept
title: REALM
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Retrieval-Augmented Language Model
  - REALM Pre-Training
tags:
  - retrieval
  - pre-training
  - dense-retrieval
related_ids:
  - concept-rag
  - concept-dense-passage-retrieval
  - concept-embedding-based-retrieval
source_ids:
  - source-guu-2020
---

## Overview

REALM (Guu et al., Google Research, 2020) is a retrieval-augmented language model that trains the knowledge retriever end-to-end during pre-training. The key innovation is using masked language modeling (MLM) as the training signal: when the model must predict a masked token, it first retrieves documents from a large corpus, then predicts the token conditioned on those documents. The retriever is updated via gradients that flow back through this retrieval step.

Prior to REALM, retrievers were either trained with supervised question–answer labels or left as unsupervised components (e.g., BM25). REALM demonstrated that a retriever can learn what documents are useful for language understanding without any labeled retrieval data — only the MLM objective is needed. At ICML 2020, it outperformed previous state-of-the-art methods by 4–16% on Open-QA benchmarks.

## Key Properties

- **End-to-end pre-training:** Retriever gradients flow from the MLM loss through the document selection step using an expectation over all documents
- **Millions of documents in the index:** The retriever considers the full Wikipedia corpus (~13 million passages) at each pre-training step
- **Asynchronous index refresh:** The document encoder is updated periodically (not continuously) to keep the MIPS index tractable during training
- **Salient span masking:** Masks named entities and dates rather than random tokens, encouraging retrieval of factual knowledge
- **4–16% improvement over prior methods:** Demonstrated on Natural Questions, WebQuestions, and CuratedTREC open versions

## Connections

REALM is the conceptual predecessor to the [[rag|RAG]] paper (Lewis et al., 2020). Where RAG fine-tunes a retriever that was pre-trained separately, REALM integrates retrieval into the pre-training objective itself — a more deeply coupled architecture. [[dense-passage-retrieval|Dense Passage Retrieval]] (Karpukhin et al., 2020) is complementary: DPR trains the retriever on supervised QA pairs, while REALM trains it unsupervised via language modeling.

[[self-rag|Self-RAG]] (Asai et al., 2023) revisits REALM's self-supervised spirit but at inference time: rather than pre-training with retrieval, it teaches the model to decide dynamically when retrieval would help during generation. Both share the insight that retrieval should be selective and task-driven rather than always-on.

VaultMind's vault functions as REALM's non-parametric memory: a corpus of structured documents the agent retrieves from before responding. The REALM pre-training paradigm suggests that agents deeply integrated with a retrieval corpus from the start — not bolted on at inference time — may develop stronger retrieval intuitions. This is a consideration for any future VaultMind agent fine-tuning work.
