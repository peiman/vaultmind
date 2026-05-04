---
id: concept-tree-of-thought
type: concept
title: Tree-of-Thoughts
created: 2026-04-29
aliases:
  - ToT
  - Tree of Thoughts
tags:
  - llms
  - deep-learning
  - prompting
  - reasoning
  - search
related_ids:
  - concept-chain-of-thought
  - concept-reflexion
  - concept-react
source_ids:
  - source-yao-2023-tot
---

## Overview

Tree-of-Thoughts (ToT) is a generalization of [[chain-of-thought|Chain-of-Thought]] prompting from a single linear reasoning chain to an explicit search tree over partial reasoning states. At each step, the model proposes multiple candidate "thoughts" (partial solutions or sub-steps), self-evaluates each one's promise, and a search algorithm (BFS, DFS, beam) decides which branches to expand. The framework lets LLMs backtrack and explore alternatives — a basic capability of deliberate human problem solving that CoT cannot express.

Introduced by [[source-yao-2023-tot|Yao et al. 2023]], ToT delivered large gains on tasks where a single chain easily goes wrong: Game of 24 (4% with CoT → 74% with ToT), creative writing with constraint satisfaction, and 5×5 mini-crosswords. The paper is the proximate ancestor of inference-time-search methods and motivated much of the subsequent reasoning-model line of work.

## How It Works

ToT decomposes a problem into a search over a tree whose nodes are partial solutions ("thoughts"). The framework requires four task-specific design choices:

1. **Thought decomposition.** Define what a single thought is (one equation in Game of 24, one paragraph in writing, one word placement in crosswords). Granularity matters — too small explodes the tree, too large defeats the point.

2. **Thought generator.** A prompt that, given the current state, proposes k candidate next thoughts. Sampled (diverse) or enumerated.

3. **State evaluator.** A prompt that, given a state, returns either a value (sure / likely / impossible) or pairwise rankings. This is the self-evaluation step that lets ToT prune.

4. **Search algorithm.** BFS keeps the top-k states at each depth; DFS explores depth-first with backtracking on low-evaluator states; beam search is in between. The choice depends on tree shape and evaluation cost.

The whole loop runs as a Python program that orchestrates LLM calls — ToT is fundamentally a meta-prompting framework, with the LLM as the proposer and evaluator and a classical search algorithm as the controller.

## Recent Developments

- **Graph-of-Thoughts (Besta et al. 2023)** — generalize the tree to a DAG, allowing reasoning paths to merge and reuse intermediate results.
- **Reasoning via Planning (RAP)** — combine ToT-style search with Monte Carlo Tree Search and a learned world model.
- **Inference-time-compute scaling** — the ToT insight (more search → better answers) became a central scaling axis. o1, o3, DeepSeek-R1, and Gemini Thinking effectively internalize ToT-style search via RL on process rewards, eliminating the need for explicit external orchestration.
- **Verifier-guided beam search** — production systems use a learned verifier (process reward model) instead of LLM self-evaluation, which is more reliable.
- **Best-of-N + verifier** — a degenerate ToT (depth-1, wide branching) that is competitive on many tasks and far simpler.

## Connections

ToT extends [[chain-of-thought|CoT]] along the explore-vs-exploit dimension. CoT is a single greedy path; self-consistency is many independent paths; ToT is a structured search over branching paths with backtracking. The progression maps onto System-1 (CoT) vs. System-2 (ToT) cognition.

ToT shares structure with [[reflexion|Reflexion]] (self-evaluation feedback loop) and [[react|ReAct]] (interleaved reasoning and action), and all three share the conceptual root of giving the LLM more inference-time compute organized around a search/feedback structure rather than a single forward generation.

For VaultMind, ToT suggests a query-decomposition pattern: hard questions about the vault decompose into sub-questions, each of which retrieves a sub-context, and a controller prunes promising branches before composing the final answer.
