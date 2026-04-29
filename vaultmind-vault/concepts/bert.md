---
id: concept-bert
type: concept
title: BERT
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Bidirectional Encoder Representations from Transformers
tags:
  - llms
  - deep-learning
  - pretraining
  - transformers
  - encoder-only
related_ids:
  - concept-gpt
  - concept-transformer
  - concept-attention-mechanism
  - concept-dense-passage-retrieval
  - concept-colbert
source_ids:
  - source-devlin-2018
---

## Overview

BERT (Bidirectional Encoder Representations from Transformers), introduced by [[source-devlin-2018|Devlin et al. 2018]] at Google AI Language, is the foundational encoder-only transformer language model. It is trained on raw text with two objectives: masked language modeling (MLM) — predict randomly masked tokens given both left and right context — and next-sentence prediction (NSP). The bidirectional context distinguishes BERT from prior left-to-right LMs (ELMo, GPT-1) and produces representations far better suited to classification, tagging, and retrieval tasks.

BERT-large set new state-of-the-art on 11 NLP benchmarks at release (GLUE, SQuAD, MultiNLI, SWAG) and established the "pretrain then fine-tune" recipe that dominated NLP from 2018 through the LLM era. While generative LLMs occupy more public attention now, BERT-lineage encoders remain the workhorses of every dense retrieval and embedding system in production today.

## How It Works

**Architecture.** A stack of transformer encoder blocks (no decoder, no causal masking). BERT-base has 12 layers, 768 hidden dim, 12 attention heads (~110M parameters). BERT-large has 24 layers, 1024 hidden dim, 16 heads (~340M parameters). All [[attention-mechanism|attention]] is fully bidirectional.

**Tokenization.** WordPiece subword tokenizer with a 30K vocabulary. A special `[CLS]` token prepended to every input serves as a sequence-level representation; `[SEP]` separates segments.

**Pretraining objectives:**
- **Masked Language Modeling (MLM).** Randomly select 15% of tokens; replace 80% with `[MASK]`, 10% with a random token, 10% unchanged. Predict the original tokens from their bidirectional context. The mask-with-probability-not-1 trick is needed because `[MASK]` never appears at fine-tuning time.
- **Next Sentence Prediction (NSP).** Binary classification: is sentence B the actual continuation of sentence A, or a random other sentence? Later research (RoBERTa) showed NSP is unnecessary, but the dual-segment input format it required (`[SEP]`, segment IDs) carried over.

**Fine-tuning.** For downstream tasks, attach a small head (linear layer) and fine-tune end-to-end. For classification, use the `[CLS]` token's hidden state; for token-level tasks (NER, tagging) use per-token hidden states; for span tasks (QA) predict start/end indices.

## Recent Developments

The BERT lineage has many descendants:

- **RoBERTa (Liu et al. 2019)** — drop NSP, train longer on more data, dynamic masking; substantially better than BERT.
- **ALBERT (Lan et al. 2019)** — parameter-sharing across layers for efficiency.
- **DeBERTa (He et al. 2020)** — disentangled position/content attention; the strongest pure-encoder model for many years.
- **ELECTRA (Clark et al. 2020)** — replace MLM with a more sample-efficient discriminative objective (real-vs-replaced token detection).
- **Sentence-BERT / SBERT (Reimers 2019)** — siamese fine-tuning for sentence embeddings.
- **DPR ([[dense-passage-retrieval|Dense Passage Retrieval]], Karpukhin 2020)** — BERT bi-encoder for open-domain question answering retrieval.
- **[[colbert|ColBERT]] (2020)** — token-level late interaction over BERT representations.
- **BGE-M3, E5, GTE, mxbai** — modern multilingual / multi-task BERT-lineage embedding models that power most production retrieval systems, including VaultMind.

## Connections

BERT and [[gpt|GPT]] are the two great branches of the [[transformer|transformer]] family tree — encoder-only vs decoder-only, bidirectional MLM vs left-to-right LM, fine-tune-per-task vs prompt-and-generate. They were once direct competitors; today they specialize. BERT-lineage encoders dominate where you need a vector representation (retrieval, classification, semantic search). GPT-lineage decoders dominate where you need to generate text (chat, completion, reasoning).

For VaultMind specifically, BERT is everywhere upstream: BGE-M3 (the embedding model VaultMind uses for indexing) is a BERT-lineage encoder, and every dense retrieval architecture in the codebase ([[dense-passage-retrieval|DPR]], [[colbert|ColBERT]], [[hybrid-search|hybrid search]]) traces directly to it.

The cognitive analog is the bidirectional context-fitting that humans do when reading: you don't strictly process left-to-right; you continuously revise interpretations of earlier words based on later ones. MLM trains exactly this.
