---
id: concept-query-expansion
type: concept
title: Query Expansion
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Query Reformulation
  - Query Rewriting
tags:
  - retrieval
  - preprocessing
related_ids:
  - concept-hyde
  - concept-rag
  - concept-dense-passage-retrieval
source_ids: []
---

## Overview

Query expansion is the practice of augmenting or reformulating a retrieval query to improve recall by bridging the vocabulary gap between how a user phrases a question and how relevant documents are written. The vocabulary mismatch problem is a fundamental challenge in information retrieval: a document about "cardiac arrest" may use that term exclusively, while a user querying "heart attack" will miss it with exact-match systems. Query expansion addresses this by enriching the query with synonyms, related terms, paraphrases, or alternative framings of the same information need.

Query expansion techniques span classical IR methods and modern neural approaches:

**Pseudo-relevance feedback (PRF)** is a classical approach: issue the original query, retrieve the top-k documents, extract the most discriminative terms from those documents, and re-issue an expanded query that includes those terms. The assumption is that the top-k initial results are relevant (the "pseudo" in PRF), so their vocabulary is informative. PRF is effective but fragile — if the initial top-k results are noisy, the expanded query may drift away from the original intent.

**Synonym and thesaurus expansion** adds lexical synonyms to the query from a knowledge base (e.g., WordNet) or domain-specific thesaurus. This is deterministic and interpretable but limited to synonymy and does not capture conceptual relatedness or paraphrase.

**LLM-based query rewriting** uses a language model to generate alternative phrasings of the query. The model can produce paraphrases, expand abbreviations, add context, or generate multiple diverse query variants. The original query plus several LLM-generated variants are issued in parallel, and results are merged (e.g., via reciprocal rank fusion). This approach generalizes across domains without requiring a curated thesaurus.

**HyDE (Hypothetical Document Embedding)** takes a different approach: instead of expanding the query, a language model generates a hypothetical document that would answer the query, then that hypothetical document is embedded and used as the retrieval query. This exploits the fact that the embedding space for documents and queries may be better aligned when both sides are document-shaped text. See [[hyde|HyDE]] for full details.

**Multi-query retrieval** generates N paraphrase queries from the original using an LLM and retrieves results for each, then deduplicates and merges the result sets. This increases recall at the cost of N× retrieval calls and requires a merge strategy to produce a final ranked list.

## Key Properties

- **Vocabulary mismatch is fundamental:** BM25 and TF-IDF are acutely sensitive to vocabulary mismatch because they rely on exact term overlap. Dense embedding retrieval partially addresses this through semantic similarity in embedding space, but even dense retrievers can miss synonymous content when the synonym occupies a different region of the embedding manifold.
- **Expansion vs. reformulation:** Expansion adds terms to the original query; reformulation replaces it with a different expression of the same intent. Both serve recall, but reformulation can also improve precision by correcting ambiguous or under-specified original queries.
- **Latency cost:** Multi-query expansion multiplies the number of retrieval calls. Systems that generate 3–5 query variants and merge results trade latency for recall. Merging strategies (union, reciprocal rank fusion, score-based combination) each have different precision-recall profiles.
- **Round-trip expansion:** Generating expanded queries using an LLM adds an LLM call before retrieval. For latency-sensitive applications, this adds 50–500ms depending on model size and infrastructure.

## Connections

[[hyde|HyDE]] is a specialized query expansion technique where the query is expanded into a full hypothetical document rather than additional search terms. HyDE and multi-query expansion are complementary and can be combined.

Query expansion improves recall at the first-stage retrieval level. It can be combined with [[reranking|reranking]]: the expanded query retrieves a larger, higher-recall candidate set, which the cross-encoder reranker then filters down to high-precision results.

For [[dense-passage-retrieval|DPR]]-style retrieval, query expansion addresses vocabulary mismatch even in embedding space — if a bi-encoder was not trained on the exact domain vocabulary, expanding to synonyms or generating a hypothetical document can improve recall significantly.

For VaultMind, query expansion is a natural fit at the agent layer. An agent searching VaultMind for notes about "memory decay" would benefit from also searching for "forgetting curve", "retention loss", and "Ebbinghaus effect" — related terms that may appear in vault notes without the exact phrase "memory decay". VaultMind's existing alias resolution is a static form of query expansion: a note's defined aliases are indexed as alternative search terms for that note. Dynamic query expansion goes further by generating expansion terms at query time based on the specific query, rather than relying on pre-defined aliases. For LLM-powered agents using VaultMind, the agent itself could generate 2–3 query expansions before issuing search calls, or VaultMind could expose a query expansion option that internally calls a lightweight model.
