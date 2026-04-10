---
id: source-honda-2024
type: source
title: "Honda et al. — Human-Like Remembering and Forgetting in LLM Agents (2024)"
created: 2026-04-09
vm_updated: 2026-04-09
url: "https://dl.acm.org/doi/10.1145/3613904.3642135"
aliases:
  - Honda 2024
  - ACT-R LLM agents paper
tags:
  - act-r
  - agent-memory
  - llm-agents
related_ids:
  - concept-base-level-activation
  - concept-temporal-activation-for-intermittent-systems
  - concept-act-r
---

# Honda et al. — Human-Like Remembering and Forgetting in LLM Agents (2024)

Honda et al. present an LLM dialogue agent whose memory system is governed directly by ACT-R's base-level activation equation. The system incorporates four ACT-R factors: temporal decay (power-law over wall-clock time), frequency (more retrievals raise activation), semantic similarity (embedding cosine between stored memory and query), and stochastic noise (Gaussian perturbation for realistic variability). Memories that fall below the activation threshold are effectively forgotten; the agent cannot retrieve them.

Human evaluation at HAI 2024 (ACM) confirms that the agent produces human-like remembering and forgetting patterns. Participants rated the memory behavior as more natural than baseline recency-only or frequency-only approaches.

This is the closest direct prior art to VaultMind's activation scoring. The key difference: Honda et al. use standard wall-clock time throughout. They do not adjust for intermittent usage, session structure, or idle compression. A user who accesses the system monthly would experience the same decay trajectory as a daily user at the same wall-clock timestamps — making long-inactive memories unretrievable regardless of their importance. VaultMind's [[concept-temporal-activation-for-intermittent-systems|Temporal Activation]] model extends their approach with the gamma parameter to compress idle time, preserving retrievability for intermittent CLI usage patterns.

Citation: Honda, Y. et al. (2024). Human-Like Remembering and Forgetting in LLM Agents. In *Proceedings of the CHI Conference on Human Factors in Computing Systems (HAI track)*. ACM. https://dl.acm.org/doi/10.1145/3613904.3642135
