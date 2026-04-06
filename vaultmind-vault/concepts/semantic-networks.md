---
id: concept-semantic-networks
type: concept
title: Semantic Networks
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Knowledge Graphs
  - Concept Maps
tags:
  - cognitive-science
  - knowledge-representation
  - graph-theory
related_ids:
  - concept-spreading-activation
  - concept-associative-memory
source_ids:
  - source-collins-quillian-1969
---

## Overview

Semantic networks are graph structures where nodes represent concepts and edges represent relationships between them. Originally proposed by Quillian (1967) as a model of human semantic memory, they became foundational in both cognitive science and artificial intelligence.

In cognitive science, semantic networks model how conceptual knowledge is organized — "a canary is a bird" is stored as an IS-A edge, and properties like "can fly" are inherited through the graph structure rather than stored redundantly.

In AI, semantic networks evolved into knowledge graphs (Freebase, Wikidata, Google Knowledge Graph) — large-scale typed property graphs used for structured reasoning.

## Key Properties

- **Typed edges:** Relationships carry semantic labels (IS-A, HAS-PART, CAUSES, RELATED-TO)
- **Inheritance:** Properties propagate through IS-A hierarchies
- **Intersection search:** Finding connections between two nodes by traversing the graph from both ends
- **Cognitive economy:** Information stored at the highest applicable level in the hierarchy

## Connections

VaultMind's graph model is a semantic network where note types correspond to node categories and edge types (`explicit_link`, `explicit_relation`, `tag_overlap`, etc.) provide typed relationships. Unlike classical semantic networks, VaultMind distinguishes between author-explicit edges (high confidence) and system-inferred edges (medium/low confidence).

The [[entity-resolution|Entity Resolution]] system maps human-readable references (titles, aliases) to canonical node IDs — analogous to how semantic networks resolve surface-form descriptions to concept nodes.
