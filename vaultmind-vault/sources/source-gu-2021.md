---
id: source-gu-2021
type: source
title: "Gu, Goel & Ré. Efficiently Modeling Long Sequences with Structured State Spaces (S4, 2021)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2111.00396"
tags:
  - deep-learning
  - state-space-models
  - architecture
related_ids:
  - concept-mamba-state-space-models
---

# Gu, Goel & Ré — S4 (2021)

Gu, Goel, and Ré (Stanford Hazy Research) introduced S4, a structured state-space sequence model that parameterizes a linear time-invariant SSM via a special HiPPO-derived state matrix. The HiPPO theory gives the SSM a principled long-range memory: its hidden state optimally compresses the input history under a chosen measure. With the structured (DPLR) parameterization, S4 admits an efficient O(N log N) FFT-based computation during training and O(1) per-step recurrence during inference.

S4 set new state-of-the-art on the Long Range Arena benchmark and was the first deep SSM to be competitive at long-sequence language tasks. It is the conceptual ancestor of [[source-gu-dao-2023|Mamba]] and the entire modern SSM lineage (S5, H3, Hyena, GSS).
