---
id: concept-associative-memory
type: concept
title: Associative Memory
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Content-Addressable Memory
  - Associative Recall
tags:
  - cognitive-science
  - memory-systems
  - foundational
related_ids:
  - concept-spreading-activation
  - concept-semantic-networks
  - concept-encoding-specificity
source_ids: []
---

## Overview

Associative memory is a memory system where retrieval is driven by content similarity or learned associations rather than by address or index. Given a partial or related cue, associative memory returns the stored pattern most closely matching that cue.

In cognitive science, associative memory refers to the human ability to link related concepts, experiences, and stimuli — hearing a song triggers a memory of a place, which triggers a memory of a person. The strength of these associations varies based on co-occurrence frequency, emotional salience, and recency of activation.

In computer science, associative memory has formal implementations: Hopfield networks (energy-based pattern completion), Bidirectional Associative Memory (BAM), and modern transformer attention mechanisms (which implement a soft form of content-addressable lookup).

## Key Properties

- **Content-addressed:** Retrieval is based on similarity to a query pattern, not on a location or key
- **Pattern completion:** Partial inputs can retrieve complete stored patterns
- **Graceful degradation:** Noisy or incomplete cues still produce useful retrievals
- **Interference:** Similar stored patterns can compete, causing retrieval errors

## Connections

VaultMind uses the term "associative memory" to describe its graph-based retrieval system. Strictly speaking, VaultMind implements **structured graph traversal** rather than true associative memory — it requires a fully specified seed note (an ID), not a partial content cue. The [[spreading-activation|Spreading Activation]] metaphor is closer to VaultMind's actual behavior.

True associative memory in the AI sense would require [[embedding-based-retrieval|Embedding-Based Retrieval]] — using vector similarity to find semantically related content from partial cues.
