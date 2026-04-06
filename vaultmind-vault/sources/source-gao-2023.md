---
id: source-gao-2023
type: source
title: "Gao, L., Ma, X., Lin, J., & Callan, J. (2023). Precise Zero-Shot Dense Retrieval without Relevance Labels. ACL 2023."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2212.10496"
aliases:
  - Gao 2023
  - HyDE paper
tags:
  - retrieval
  - zero-shot
related_ids:
  - concept-hyde
  - concept-dense-passage-retrieval
  - concept-rag
---

# Gao, Ma, Lin, & Callan — HyDE (ACL 2023)

Gao, Ma, Lin, and Callan introduce Hypothetical Document Embeddings (HyDE), a zero-shot dense retrieval approach that addresses the query-document asymmetry problem. Standard dense retrieval embeds queries and documents with the same model, but queries are typically short and abstract while documents are long and concrete, creating a geometric mismatch in embedding space. HyDE resolves this by using an LLM to generate a hypothetical document — a plausible, document-length answer to the query — and then embedding that hypothetical document to retrieve real documents by similarity.

The LLM's hypothetical document may contain factual errors, but its embedding nonetheless captures the vocabulary, style, and topical signals of a relevant real document. The resulting embedding is geometrically closer to real relevant documents than the raw query embedding, improving retrieval precision without any labeled training data.

Evaluation uses BEIR (heterogeneous retrieval benchmark) and TREC DL, comparing HyDE (using Contriever as encoder and InstructGPT for generation) against Contriever in its zero-shot setting. HyDE consistently outperforms the Contriever baseline across BEIR tasks. The paper also shows that HyDE with a stronger supervised encoder (GTR-XXL) further improves results, indicating the approach stacks with better base models.

## Key Findings

- HyDE outperforms zero-shot Contriever on 11 of 18 BEIR benchmark tasks, with particularly strong gains on biomedical (TREC-COVID, SciFact) and technical domains where query-document vocabulary mismatch is large
- Factual correctness of the hypothetical document is not required: ablations show that factually incorrect hypothetical documents still improve retrieval over raw query embeddings, confirming that geometric alignment, not factual accuracy, drives gains
- HyDE + GTR-XXL (supervised encoder) achieves competitive performance with fully supervised retrievers on BEIR, demonstrating that zero-shot generation can partially substitute for relevance label collection
- Prompt engineering for the hypothetical document matters: prompts that specify the document type (e.g., "write a Wikipedia passage about...") improve results over generic completion prompts
- The approach adds one LLM forward pass per query, which is the only additional computational cost over standard dense retrieval

## Relevance to VaultMind

HyDE offers VaultMind a practical, zero-shot path to better retrieval without requiring labeled training data from users' vaults. When a user queries their vault, generating a hypothetical ideal note and then finding the closest real notes bridges the stylistic gap between how questions are asked and how personal notes are written. This is particularly valuable in VaultMind's context because personal notes often use shorthand, idiosyncratic terminology, or implicit context that makes them hard to match via raw query embeddings. A [[context-pack|Context Pack]] generation pipeline incorporating HyDE could surface deeply buried but highly relevant notes that standard embedding search would miss.
