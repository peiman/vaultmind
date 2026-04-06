---
id: concept-spreading-activation
type: concept
title: Spreading Activation
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Activation Spreading
  - Collins-Loftus Model
tags:
  - cognitive-science
  - retrieval
  - associative-memory
related_ids:
  - concept-associative-memory
  - concept-semantic-networks
source_ids:
  - source-collins-loftus-1975
---

## Overview

Spreading activation is a method for searching associative networks, semantic networks, or neural networks. When a concept is activated (retrieved, primed, or attended to), activation spreads outward along connected edges to related concepts, raising their accessibility.

The strength of activation decreases with distance from the source and is modulated by the weight (strength) of each connection. Concepts that receive activation from multiple sources simultaneously experience summation, making them more likely to reach the threshold for conscious retrieval.

## Key Properties

- **Decay with distance:** Activation attenuates as it propagates through the network. Nodes farther from the source receive less activation.
- **Fan-out effect:** Nodes with many outgoing connections distribute activation more thinly across each connection.
- **Intersection detection:** When two searches propagate simultaneously (e.g., from two priming concepts), nodes at the intersection receive converging activation — this is how relatedness is detected.
- **Threshold-based retrieval:** A node becomes "retrieved" only when its accumulated activation exceeds a threshold.

## Connections

Spreading activation is the primary computational metaphor behind VaultMind's `memory recall` command. The `--depth` parameter controls how far activation spreads, and `--min-confidence` acts as a threshold filter. However, VaultMind's implementation is a deterministic BFS traversal, not a continuous activation model — see [[associative-memory|Associative Memory]] for the distinction.

The [[act-r|ACT-R]] architecture formalizes spreading activation with precise mathematical equations for base-level activation, associative strength, and decay.
