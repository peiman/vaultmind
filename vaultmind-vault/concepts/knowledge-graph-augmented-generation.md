---
id: concept-knowledge-graph-augmented-generation
type: concept
title: Knowledge Graph-Augmented Generation
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - KGAG
  - KG-Augmented Generation
  - Graph-Augmented LLM
tags:
  - knowledge-graph
  - retrieval
  - rag-variant
related_ids:
  - concept-graph-rag
  - concept-rag
  - concept-semantic-networks
  - concept-entity-resolution
source_ids: []
---

## Overview

Knowledge Graph-Augmented Generation (KGAG) is a family of approaches that augment LLM generation with structured knowledge graphs rather than flat text passages. Instead of retrieving document chunks via vector similarity, KGAG retrieves structured triples (entity-relation-entity) or subgraphs and conditions generation on that structured context.

The core insight is that knowledge graphs encode *relationships explicitly* — a triple like `(Alan Turing, invented, Turing machine)` is semantically precise in a way that embedding-based similarity cannot match. This makes KGAG especially powerful for relational and multi-hop questions.

Representative approaches:
- **KG-FiD:** Extends Fusion-in-Decoder (FiD) by prepending KG triples to retrieved passages, combining structured and unstructured evidence.
- **Think-on-Graph (ToG):** Uses the LLM as a reasoning agent that iteratively traverses a KG (e.g., Freebase, Wikidata), selecting which edges to follow at each hop.
- **GraphRAG:** Builds a KG from source documents at index time, then retrieves community summaries at query time (see [[graph-rag|GraphRAG]]).

## Key Properties

- **Structured retrieval unit:** Triples or subgraphs, not text passages
- **Multi-hop reasoning:** Graph traversal naturally handles chains of reasoning ("Who is the spouse of the inventor of X?")
- **Reduced hallucination:** Grounded in explicit, verifiable facts from the KG
- **Relational precision:** Typed edges carry semantic meaning that embedding similarity approximates only loosely
- **Cold-start limitation:** Requires a pre-built KG; novel entities not in the graph cannot be retrieved

## Connections

VaultMind is a KGAG system by design. The vault is a typed property graph of notes connected by explicit and inferred edges; the [[context-pack|Context Pack]] assembles subgraph neighborhoods for agent consumption. VaultMind's edge-type hierarchy (explicit_relation > explicit_link > inferred) mirrors the confidence weighting that KGAG systems apply to KG triples vs. extracted relations.

The multi-hop strength of KGAG maps directly to VaultMind's graph traversal depth parameter: higher depth captures more distant relational context, enabling reasoning over chains of note relationships that flat retrieval would miss entirely.

Where KGAG typically queries large curated graphs (Wikidata, Freebase), VaultMind operates on a personal knowledge graph — smaller but more precisely annotated by the author who understands their own domain.
