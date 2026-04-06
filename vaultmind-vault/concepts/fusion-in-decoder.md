---
id: concept-fusion-in-decoder
type: concept
title: Fusion-in-Decoder
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - FiD
  - Fusion in Decoder
tags:
  - retrieval
  - generative-qa
  - architecture
related_ids:
  - concept-rag
  - concept-dense-passage-retrieval
  - concept-self-rag
source_ids:
  - source-izacard-grave-2021
---

## Overview

Fusion-in-Decoder (Izacard & Grave, Facebook AI Research, 2021) addresses a key limitation of early retrieve-and-read systems: how to effectively use many retrieved passages at once. The approach is architecturally simple — each retrieved passage is concatenated with the question and encoded independently by a T5 encoder, producing one encoded representation per passage. All encoded representations are then concatenated and passed jointly to the decoder, which attends over all of them via standard cross-attention.

This contrasts with "early fusion" (concatenating all passages before encoding, which hits sequence length limits) and with "extract-then-generate" approaches that pick one passage before generating. By encoding passages independently and fusing only in the decoder, FiD scales to 100 retrieved passages without the quadratic attention cost of encoding everything together. Published at EACL 2021, it achieved state of the art on Natural Questions and TriviaQA.

## Key Properties

- **Independent encoding:** Each (question, passage) pair is encoded separately — no inter-passage attention during encoding
- **Decoder-side fusion:** The decoder attends over all encoded representations simultaneously via cross-attention
- **Scales with passage count:** Performance improves as more passages are retrieved, with gains observed up to 100 passages
- **Simple implementation:** Requires no architectural changes beyond striding encoded representations into the decoder's memory
- **SOTA on NQ and TriviaQA:** Outperformed all contemporary extractive and generative approaches at EACL 2021

## Connections

FiD builds directly on [[dense-passage-retrieval|Dense Passage Retrieval]] — in the original paper, DPR is used as the retriever feeding passages into the FiD reader. The combination of DPR + FiD became a standard strong baseline for open-domain QA research in 2021–2022.

[[rag|RAG]] (Lewis et al., 2020) addressed multi-passage aggregation differently: RAG-Token marginalizes over retrieved documents at the token level, effectively weighting each document's contribution to each generated token. FiD achieves similar multi-document synthesis but more efficiently through decoder cross-attention rather than per-token marginalization.

[[self-rag|Self-RAG]] (Asai et al., 2023) is conceptually downstream: it solves when to retrieve, while FiD solves how to aggregate what was retrieved. A complete retrieval pipeline might use Self-RAG's retrieval-gating logic to decide when to fetch, DPR to fetch, and FiD's fusion strategy to aggregate the results.

VaultMind currently injects retrieved note content sequentially into the prompt. FiD's independent-encode-then-fuse pattern suggests an alternative for multi-note retrieval: encode each note chunk with the query independently, then synthesize across all encoded representations in the generation step. This would be particularly relevant if VaultMind moves to a RAG-style embedding backend in v2.
