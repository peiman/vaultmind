---
id: source-devlin-2018
type: source
title: "Devlin et al. BERT: Pre-training of Deep Bidirectional Transformers for Language Understanding (2018)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/1810.04805"
tags:
  - llms
  - deep-learning
  - pretraining
  - transformers
related_ids:
  - concept-bert
---

# Devlin et al. — BERT (2018)

Devlin, Chang, Lee, and Toutanova (Google AI Language) introduced BERT, the first widely successful encoder-only bidirectional transformer pre-trained with masked language modeling (MLM) and next-sentence prediction. By predicting randomly-masked tokens conditioned on both left and right context, BERT learns bidirectional representations that earlier left-to-right models (ELMo, GPT-1) could not.

BERT-large set new state-of-the-art on 11 NLP benchmarks at release (GLUE, SQuAD, MultiNLI) and established the "pre-train then fine-tune" recipe that dominated NLP for years. It is the foundational paper of the encoder-only branch of the transformer family — the lineage of RoBERTa, DeBERTa, and the dense-retrieval encoders ([[concept-bert|BERT]] underpins DPR, ColBERT, BGE-M3, and most modern embedding models).
