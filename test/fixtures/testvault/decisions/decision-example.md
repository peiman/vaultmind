---
id: decision-example
type: decision
title: Use BGE-M3 for hybrid retrieval
status: accepted
created: 2026-04-03
tags:
  - retrieval
related_ids:
  - concept-spreading-activation
source_ids: []
---

## Context

We needed an embedding model that supports dense, sparse, and late-interaction retrieval from a single pass, to avoid running three models.

## Decision

Adopt [[BGE-M3]] as the embedding backend. It produces dense, sparse, and ColBERT vectors together, which feeds the 4-way RRF hybrid directly.

## Consequences

The build links an ONNX runtime, but a single model covers all three retrieval lanes — a worthwhile trade for the quality gain.
