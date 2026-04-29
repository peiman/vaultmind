---
id: source-yao-2023-tot
type: source
title: "Yao et al. Tree of Thoughts: Deliberate Problem Solving with Large Language Models (2023)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2305.10601"
tags:
  - llms
  - deep-learning
  - prompting
  - reasoning
related_ids:
  - concept-tree-of-thought
  - concept-chain-of-thought
---

# Yao et al. — Tree of Thoughts (2023)

Yao and colleagues (Princeton, Google DeepMind) generalized [[source-wei-2022-cot|Chain-of-Thought]] prompting from a single linear chain to an explicit search tree over partial reasoning states. At each step, the model proposes multiple candidate "thoughts" (sub-steps), self-evaluates each, and search algorithms (BFS, DFS, beam) decide which branch to expand. The framework lets LLMs backtrack and explore, mimicking deliberate System-2 problem solving.

ToT delivered large gains on tasks where a single chain often goes wrong: Game of 24 (4% → 74%), creative writing, and 5×5 crosswords. The paper is the proximate ancestor of inference-time-search methods, including the verifier-guided search lines that motivate o1/o3-style training.
