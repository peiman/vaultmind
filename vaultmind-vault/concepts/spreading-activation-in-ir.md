---
id: concept-spreading-activation-in-ir
type: concept
title: "Spreading Activation in Information Retrieval"
status: active
created: 2026-04-09
tags:
  - retrieval
  - information-retrieval
  - spreading-activation
related_ids:
  - concept-spreading-activation
  - concept-hybrid-search
  - concept-rag
  - concept-base-level-activation
---

## Overview

Spreading activation, originally a cognitive science model for semantic memory retrieval (Collins & Loftus, 1975), has been directly applied to information retrieval systems. Crestani (1997) surveyed these applications, showing that document networks can be modeled as associative networks where activation spreads from query terms through document-term and document-document links.

The core principle transfers directly: a query activates matching nodes, activation spreads along weighted edges (term co-occurrence, citation links, hyperlinks), and the most activated documents are retrieved. This captures associative relevance that keyword matching alone misses.

## Key Properties

- Associative retrieval: Documents related to the query through indirect links are surfaced, not just direct keyword matches
- Weighted propagation: Edge weights control how much activation spreads — stronger associations carry more signal
- Decay with distance: Activation diminishes with each hop, preventing the entire network from activating
- Convergent evidence: Documents reached by multiple spreading paths receive higher cumulative activation
- Computationally bounded: BFS with a visited set and max depth prevents runaway activation

## Connection to Use-Dependent Strengthening

In IR systems, relevance feedback strengthens query-document associations — the IR analog of Hebbian learning. When a user clicks or selects a document for a query, that association is reinforced, making the document more likely to be retrieved for similar future queries (Joachims, 2002; Salton & Buckley, 1990).

VaultMind already implements graph-based spreading activation for context-pack assembly. The next step is closing the feedback loop: when a note is accessed via search or recall, the edges that led to it should be implicitly strengthened, just as synapses strengthen with use.

## Sources

- Crestani, F. (1997). Application of spreading activation techniques in information retrieval. *Artificial Intelligence Review*, 11(6), 453-482. DOI: 10.1023/A:1006569829653
- Joachims, T. (2002). Optimizing search engines using clickthrough data. *Proceedings of ACM SIGKDD*, 133-142. DOI: 10.1145/775047.775067
- Salton, G. & Buckley, C. (1990). Improving retrieval performance by relevance feedback. *JASIS*, 41(4), 288-297. DOI: 10.1002/(SICI)1097-4571(199006)41:4<288::AID-ASI8>3.0.CO;2-H
