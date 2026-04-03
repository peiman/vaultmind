---
id: person-joon-sung-park
type: person
title: "Joon Sung Park"
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Park
tags:
  - ai
  - llm-agents
  - memory-systems
related_ids:
  - concept-generative-agents
  - source-park-2023
url: "https://cs.stanford.edu/~joonsup/"
---

## About

Joon Sung Park is a PhD student at Stanford who led the Generative Agents project, published in 2023. That work demonstrated that LLM-driven agents could exhibit coherent, socially plausible long-term behavior when equipped with a structured memory architecture combining retrieval, reflection, and planning over a persistent memory stream.

## Key Contributions

Park's [[Generative Agents]] architecture is one of the clearest existence proofs that external memory systems dramatically extend LLM coherence over time. The three-layer design — retrieval weighted by recency, importance, and relevance; higher-order reflection that synthesizes patterns from raw memories; and forward planning grounded in those reflections — maps directly onto VaultMind's layered approach. His scoring formula for memory retrieval (normalized sum of the three factors) is a practical baseline for how VaultMind could rank notes in a context pack when multiple candidates compete for the context window.
