---
id: source-gu-dao-2023
type: source
title: "Gu & Dao. Mamba: Linear-Time Sequence Modeling with Selective State Spaces (2023)"
created: 2026-04-29
url: "https://arxiv.org/abs/2312.00752"
tags:
  - llms
  - deep-learning
  - state-space-models
  - architecture
related_ids:
  - concept-mamba-state-space-models
---

# Gu & Dao — Mamba (2023)

Albert Gu and Tri Dao introduced Mamba, a state-space-model architecture whose state-update parameters are themselves input-dependent (the "selective" mechanism). This fixed the central limitation of prior linear-time SSMs ([[source-gu-2021|S4]], H3, Hyena): they had no mechanism to ignore or compress irrelevant tokens, so they underperformed transformers on language. Selective SSMs recover content-dependent state filtering while keeping the linear-time, constant-memory recurrence.

Mamba models match transformers of 2× their size on language benchmarks, achieve 5× higher inference throughput, scale to one-million-token sequences, and dispense with attention and MLP blocks entirely. The paper revived state-space models as a serious transformer alternative and seeded a wave of hybrid architectures (Jamba, Zamba, Samba).
