---
id: concept-actmem
type: concept
title: ActMem (Active Memory)
created: 2026-04-09
vm_updated: 2026-04-09
aliases:
  - Active Memory
  - ActMem
tags:
  - agent-memory
  - knowledge-graph
  - causal-reasoning
related_ids:
  - concept-semantic-networks
  - concept-spreading-activation
  - concept-a-mem
source_ids: []
---

## Overview

ActMem (Alibaba, February 2026, arXiv:2603.00026) transforms dialogue history into a structured causal and semantic knowledge graph rather than storing raw conversation turns. Instead of simply recording what was said, ActMem deduces what is implied: counterfactual reasoning (what would happen if X were different), commonsense completion (what is implied but unstated), and implicit constraint deduction (what rules govern the situation).

The result is a memory that reasons over stored information, not just retrieves it. A system using ActMem can answer questions that were never explicitly stated in the dialogue because the graph captures the causal structure of the world being discussed.

## Key Properties

- Causal knowledge graph: Edges represent causal dependencies, not just associative co-occurrence
- Counterfactual reasoning: Can infer what would have been true under different circumstances
- Commonsense completion: Fills in implicit knowledge using structured reasoning over the graph
- Constraint deduction: Extracts rules and constraints from observed patterns
- Active over passive: Memory is a reasoning substrate, not an archive

## Connections

ActMem's causal edge types represent an extension of what VaultMind's `related_ids` currently captures. VaultMind edges are associative (this note is related to that note); ActMem's edges are causal (this event caused that outcome, this constraint implies that behavior).

For VaultMind's use case — a personal research knowledge base — the distinction matters for design decision notes. A [[concept-semantic-networks|Semantic Networks]] of design decisions with causal edges would let an agent reason about consequences: "if we changed the embedding model, which decisions would be invalidated?" This is beyond current VaultMind scope but ActMem points to the direction.

See [[concept-semantic-networks|Semantic Networks]] for the graph model that causal edges would extend.
