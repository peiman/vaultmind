---
id: concept-sparse-retrieval
type: concept
title: Sparse Retrieval
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - BM25 Retrieval
  - Keyword-Based Retrieval
  - Term-Based Retrieval
tags:
  - retrieval
  - sparse-retrieval
related_ids:
  - concept-dense-passage-retrieval
  - concept-rag
  - concept-embedding-based-retrieval
source_ids: []
---

## Overview

Sparse retrieval refers to information retrieval systems that represent documents and queries as sparse vectors — vectors where nearly all entries are zero — over a vocabulary of terms. The non-zero entries correspond to terms that appear in the document or query. Because most terms do not appear in any given document, these vectors are sparse: a vocabulary of 100,000 terms might yield a document vector with only 50–200 non-zero entries.

The dominant sparse retrieval method is **BM25** (Best Match 25), introduced by Robertson & Walker (1994) and extended through subsequent work on the probabilistic retrieval framework. BM25 improves on raw TF-IDF in two important ways: it applies a saturation function to term frequency (so that a term appearing 100 times contributes only marginally more than one appearing 10 times), and it normalizes for document length (so that longer documents are not automatically favored). The result is a well-calibrated relevance score that remains competitive with more complex methods on many standard benchmarks.

## Key Properties

- **Inverted index:** Sparse retrieval is implemented via an inverted index — a mapping from each term to the list of documents containing it. Query evaluation is extremely fast: only documents containing query terms are scored.
- **Interpretability:** The match between query and document is directly inspectable — you can see exactly which query terms contributed to a document's score and which terms matched.
- **No training required:** BM25 requires no training data, no GPU, and no model updates. It is fully deterministic and reproducible.
- **Vocabulary mismatch:** The fundamental weakness — a document that uses synonyms or paraphrases of the query terms receives a score of zero for those terms, even if it is highly relevant. BM25 cannot match "automobile" to "car" unless they co-occur.
- **No semantic generalization:** Sparse methods have no notion of semantic similarity; they operate purely on lexical overlap. Conceptual relevance is invisible to BM25.
- **Hybrid first stage:** Despite these weaknesses, sparse retrieval remains highly competitive as a first-stage ranker in hybrid pipelines, where a reranker or dense model provides a second pass over BM25 candidates.

## Connections

Sparse retrieval is the established baseline against which [[dense-passage-retrieval|Dense Passage Retrieval]] (DPR) is measured. DPR reported 9–19% gains over BM25 in top-20 passage recall on open-domain QA benchmarks, but these gains are task-specific; on keyword-heavy queries, BM25 often matches or exceeds dense methods. [[embedding-based-retrieval|Embedding-Based Retrieval]] addresses vocabulary mismatch by mapping queries and documents to dense continuous vectors, but at the cost of training requirements and reduced interpretability. The [[rag|RAG]] architecture commonly uses BM25 or a hybrid as the retrieval stage, with the generator providing semantic tolerance for retrieval gaps.

VaultMind v1 uses BM25 via SQLite FTS5 as its primary retrieval mechanism. Understanding sparse retrieval's strengths — speed, no training data, fully interpretable, zero infrastructure overhead — clarifies why it was the right choice for v1. Understanding its vocabulary mismatch weakness clarifies the specific gap that v2 dense retrieval addresses: two notes with semantically related but lexically disjoint content will not be surfaced by BM25 alone. The planned v2 hybrid pipeline adds dense retrieval as a complement to BM25, not a replacement, combining the precision of sparse matching with the semantic coverage of dense similarity.
