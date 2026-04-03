---
id: concept-base-level-activation
type: concept
title: Base-Level Activation
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Base-Level Learning
  - Memory Strength
tags:
  - cognitive-science
  - memory-models
  - act-r
related_ids:
  - concept-act-r
  - concept-forgetting-curve
  - concept-spreading-activation
source_ids:
  - source-anderson-schooler-1991
---

## Overview

Base-level activation is the ACT-R mechanism that determines how accessible a memory chunk is based on its history of use. The equation: `B_i = ln(sum(t_j^(-d)))` captures the principle that memories used more often and more recently are more accessible.

This is the mathematical formalization of what the [[Forgetting Curve]] describes qualitatively: memories decay with time but are strengthened by retrieval practice.

## Key Properties

- **Frequency effect:** More retrievals = higher base-level activation
- **Recency effect:** Recent retrievals contribute more than old ones (power-law decay)
- **Environmental regularity:** The equation matches statistical patterns of information need in real environments (Anderson & Schooler, 1991)
- **Combines with spreading activation:** Total activation = base-level + spreading from context + noise

## Connections

This is the strongest theoretical basis for adding retrieval frequency tracking to VaultMind. Currently, all notes have equal accessibility regardless of how often they've been queried. Adding `recall_count` and `last_recalled_at` fields (deferred to v2) would enable base-level activation scoring in [[Context Pack]] assembly.

The environmental regularity finding is particularly relevant: Anderson & Schooler showed that the statistical pattern of word appearance in newspapers, library book checkouts, and email frequency all follow the same power-law decay — the same pattern the activation equation captures.
