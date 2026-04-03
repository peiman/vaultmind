---
id: concept-act-r
type: concept
title: ACT-R
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Adaptive Control of Thought-Rational
  - ACT-R Architecture
  - Anderson's ACT-R
tags:
  - cognitive-science
  - cognitive-architecture
  - memory-models
related_ids:
  - concept-spreading-activation
  - concept-forgetting-curve
  - concept-base-level-activation
source_ids:
  - source-anderson-1983
---

## Overview

ACT-R (Adaptive Control of Thought—Rational) is a cognitive architecture developed by John Anderson that models human cognition as the interaction of modular components: declarative memory (facts), procedural memory (rules), and perceptual-motor modules.

Its memory model is the most mathematically precise formalization of human retrieval dynamics. It combines base-level activation (frequency and recency of use) with spreading activation from the current context to determine which memories are accessible at any moment.

## Key Properties

- **Base-level activation:** `B_i = ln(sum(t_j^(-d)))` where t_j are times since each prior retrieval and d is a decay parameter (~0.5). More recently and frequently accessed items have higher activation.
- **Spreading activation:** Context elements spread activation to associated items via weighted links. Total activation = base-level + spreading + noise.
- **Retrieval threshold:** Items are retrieved only if total activation exceeds threshold τ. Below threshold = retrieval failure (forgetting).
- **Retrieval latency:** `T = F * e^(-A_i)` — higher activation = faster retrieval. This is empirically validated.
- **Partial matching:** Imperfect cues still produce retrieval, with a mismatch penalty.

## Connections

ACT-R's base-level activation equation is the strongest theoretical basis for adding temporal decay to VaultMind. The expert panel discussed this: the `vm_updated` timestamp provides recency signal, but VaultMind lacks retrieval frequency tracking. Adding `recall_count` and `last_recalled_at` to the notes table (deferred to v2) would enable an ACT-R-inspired accessibility score for [[Context Pack]] ordering.

The [[Spreading Activation]] mechanism in ACT-R is more sophisticated than VaultMind's BFS traversal — it uses continuous activation levels rather than discrete depth-bounded hops.
