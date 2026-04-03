---
id: source-park-2023
type: source
title: "Park et al. Generative Agents: Interactive Simulacra of Human Behavior (2023)"
created: 2026-04-03
vm_updated: 2026-04-03
url: "https://doi.org/10.1145/3586183.3606763"
aliases:
  - Park 2023
tags:
  - ai-memory
  - agent-architecture
related_ids:
  - concept-generative-agents
  - concept-working-memory
---

# Park et al. — Generative Agents (2023)

Park et al. introduced a sandbox simulation of 25 AI agents capable of believable social behavior—planning days, forming relationships, and remembering past events. The key architecture comprises three memory streams: a **memory stream** (raw event log), a **reflection** mechanism that periodically synthesizes high-level observations, and a **planning** module that turns reflections into future actions. Retrieval is driven by a weighted score of recency, importance, and relevance.

The paper matters because it demonstrated that memory architecture—not just model size—determines the quality of agent behavior. Without the reflection and retrieval layers, agents repeat themselves and lose coherence over long horizons. The recency-weighted retrieval score directly mirrors how human working memory prioritizes recent and salient information, linking the work to cognitive models of [[Working Memory]].

For VaultMind, this paper is foundational. The memory stream maps naturally onto Obsidian's append-only daily notes, reflection maps onto periodic summarization passes, and the retrieval scoring formula informs how [[Generative Agents]] surfaces vault content. The importance weighting in particular influenced VaultMind's concept-scoring heuristics.
