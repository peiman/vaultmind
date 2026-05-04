---
id: source-honda-2025
type: source
title: "Honda, Fujita, Zempo, Fukushima — Human-Like Remembering and Forgetting in LLM Agents: An ACT-R-Inspired Memory Architecture (2025)"
created: 2026-04-09
url: "https://doi.org/10.1145/3765766.3765803"
aliases:
  - Honda 2025
  - Honda et al. 2025
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

# Honda et al. — Human-Like Remembering and Forgetting in LLM Agents (2025)

Honda, Fujita, Zempo, and Fukushima present an LLM dialogue agent whose memory system is governed directly by ACT-R's base-level activation equation. The system incorporates four ACT-R factors: temporal decay (power-law over wall-clock time), frequency (more retrievals raise activation), semantic similarity (embedding cosine between stored memory and query), and stochastic noise (Gaussian perturbation for realistic variability). Memories that fall below the activation threshold are effectively forgotten; the agent cannot retrieve them.

Human evaluation at HAI 2025 (ACM) confirms that the agent produces human-like remembering and forgetting patterns. Participants rated the memory behavior as more natural than baseline recency-only or frequency-only approaches.

This is the closest direct prior art to VaultMind's activation scoring. The key difference: Honda et al. use standard wall-clock time throughout. They do not adjust for intermittent usage, session structure, or idle compression. A user who accesses the system monthly would experience the same decay trajectory as a daily user at the same wall-clock timestamps — making long-inactive memories unretrievable regardless of their importance. VaultMind's [[temporal-activation-for-intermittent-systems|Temporal Activation]] model extends their approach with the gamma parameter to compress idle time, preserving retrievability for intermittent CLI usage patterns.

Citation: Honda, Y., Fujita, K., Zempo, K., & Fukushima, S. (2025). Human-Like Remembering and Forgetting in LLM Agents: An ACT-R-Inspired Memory Architecture. In *Proceedings of the 13th International Conference on Human-Agent Interaction (HAI '25)*. ACM. https://doi.org/10.1145/3765766.3765803
