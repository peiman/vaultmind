---
id: source-edge-2024
type: source
title: "Edge, D., et al. (2024). From Local to Global: A Graph RAG Approach to Query-Focused Summarization. arXiv:2404.16130."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2404.16130"
aliases:
  - Edge 2024
  - GraphRAG paper
tags:
  - knowledge-graph
  - retrieval
related_ids:
  - concept-graph-rag
  - concept-rag
  - concept-semantic-networks
---

# Edge et al. — GraphRAG (2024)

Edge et al. (Microsoft Research) introduced GraphRAG as a solution to a limitation of conventional RAG: standard vector-search retrieval fails at global sensemaking questions that require synthesis across an entire corpus rather than lookup of a specific fact. The paper proposes an offline pipeline that extracts entities and relationships from source documents using an LLM, builds a typed property graph, applies the Leiden community detection algorithm to partition the graph into hierarchical clusters, and generates natural-language summaries of each community at multiple granularity levels. At query time, community summaries — not raw text chunks — are retrieved and used in generation.

## Key Findings

- **GraphRAG vs. naive RAG on global questions:** For questions requiring synthesis across large corpora (~1M tokens), GraphRAG substantially outperforms conventional RAG on both comprehensiveness and diversity of the answer, as judged by LLM evaluation
- **Community hierarchy:** Multiple levels of community summarization support varying query specificity — coarse summaries for broad questions, fine summaries for focused ones
- **Cost tradeoff:** The offline indexing phase requires many LLM calls (expensive), but this cost is amortized across all subsequent queries against the same corpus
- **Open-sourced:** Microsoft released the implementation at github.com/microsoft/graphrag, accelerating adoption and follow-on research

## Relevance to VaultMind

VaultMind and GraphRAG share the same architectural principle: structured graph retrieval outperforms flat embedding retrieval for relational and synthesis queries. The key architectural difference is graph authorship — VaultMind's graph is built by humans (explicit Obsidian links and frontmatter relations), while GraphRAG's graph is built automatically by an LLM.

The community-summarization technique is an unimplemented capability in VaultMind. Strongly-connected subgraphs and tag clusters in the vault could be pre-summarized to support global vault queries without traversing every note. This would require a periodic background process (analogous to GraphRAG's indexing phase) rather than VaultMind's current on-demand traversal model.
