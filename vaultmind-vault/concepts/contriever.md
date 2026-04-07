---
id: concept-contriever
type: concept
title: Contriever
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Contrastive Retriever
  - Unsupervised Dense Retriever
tags:
  - retrieval
  - dense-retrieval
  - unsupervised
related_ids:
  - concept-dense-passage-retrieval
  - concept-hyde
  - concept-embedding-based-retrieval
source_ids:
  - source-izacard-2022
---

## Overview

Contriever (Izacard et al., 2022) is an unsupervised dense retriever trained using contrastive learning, specifically the MoCo (Momentum Contrast) framework with document cropping as its augmentation strategy. The core insight is that two random spans cropped from the same document should be close in embedding space — they discuss the same topic — while spans from different documents should be far apart. No relevance labels, no question-answer pairs, no human annotation of any kind is required.

The training objective treats each document crop as a positive pair with other crops from the same source document, and uses a large negative queue (from MoCo) to push embeddings of different-document spans apart. The result is a dense retriever that learns to embed text such that topically related passages cluster together, trained entirely on unlabeled text (Wikipedia + CCNet).

Despite having no access to labeled retrieval pairs, Contriever achieves strong zero-shot retrieval performance across BEIR benchmark tasks, competitive with supervised models on out-of-domain evaluation. It became the standard embedding backbone for subsequent work including HyDE. arXiv:2112.09118.

## Key Properties

- **Unsupervised training:** No relevance labels, no QA pairs, no human annotation. Trained on unlabeled document crops using contrastive learning.
- **MoCo framework:** Uses a momentum encoder and large negative queue to provide stable contrastive learning with a large effective batch size of negatives.
- **Random cropping augmentation:** Two independent random spans from the same document form a positive pair. Simple, scalable, and domain-agnostic.
- **Strong zero-shot performance:** On BEIR, Contriever outperforms BM25 on several out-of-domain tasks and closes much of the gap with supervised dense retrievers like DPR on tasks without domain-specific training data.
- **Foundation for HyDE:** HyDE uses Contriever as its embedding backbone, showing that the unsupervised encoder is expressive enough for the hypothetical document matching task.
- **Domain-agnostic:** Because it requires no labeled data, Contriever can be applied directly to new domains without fine-tuning, including personal knowledge bases.

## Connections

Contriever is a direct improvement over [[dense-passage-retrieval|DPR]] in the unsupervised regime. DPR achieves higher absolute performance when trained on in-domain labeled data, but Contriever matches or exceeds it out-of-domain due to its unsupervised training regime.

The relationship to [[hyde|HyDE]] is foundational: HyDE's zero-shot retrieval gains are demonstrated with Contriever as the encoder. The combination shows that unsupervised embeddings + generation-based query expansion can match supervised retrievers without labeled data.

[[embedding-based-retrieval|Embedding-based retrieval]] is the broader category of which Contriever is one instance, specifically the unsupervised branch.

For VaultMind, Contriever is a candidate embedding model for v2 semantic retrieval. Its unsupervised nature means it works without fine-tuning on vault-specific data — a critical property since VaultMind must work across diverse personal vaults with no shared labeled retrieval corpus. A user's vault notes could be indexed with Contriever embeddings immediately, without any vault-specific training, providing high-quality semantic search out of the box.
