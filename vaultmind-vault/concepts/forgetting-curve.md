---
id: concept-forgetting-curve
type: concept
title: Forgetting Curve
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Ebbinghaus Curve
  - Memory Decay
  - Retention Curve
tags:
  - cognitive-science
  - memory-decay
  - retrieval
related_ids:
  - concept-spacing-effect
  - concept-act-r
source_ids:
  - source-ebbinghaus-1885
---

## Overview

The forgetting curve, first described by Hermann Ebbinghaus in 1885, shows that memory retention decays exponentially over time following initial learning. Without reinforcement, approximately 50% of newly learned information is forgotten within the first hour, and roughly 70% within 24 hours.

The mathematical form is typically: `R(t) = e^(-t/S)` where R is retention, t is time since learning, and S is the stability of the memory (influenced by encoding strength, emotional salience, and number of prior retrievals).

## Key Properties

- **Exponential decay:** Retention drops sharply at first, then levels off — early review has the highest impact
- **Stability increases with retrieval:** Each successful retrieval strengthens the memory trace, increasing S and flattening the curve
- **Individual variation:** Forgetting rates vary significantly across individuals and material types
- **Not truly "forgotten":** Decayed memories often remain as latent traces that can be reactivated with less effort than original learning (savings effect)

## Connections

VaultMind v1 has no forgetting mechanism — all indexed content remains equally accessible regardless of age or access frequency. The expert panel (Session 02) debated whether to add temporal decay to retrieval ranking. The consensus was to defer decay to v2, but the [[Context Pack]] algorithm's reliance on `vm_updated` timestamps introduces an implicit recency bias.

The [[ACT-R]] architecture implements forgetting via base-level activation decay: `B_i = ln(sum(t_j^(-d)))` where t_j are the times since each prior retrieval and d is a decay parameter.
