---
id: source-yao-2023
type: source
title: "Yao, S., et al. (2023). ReAct: Synergizing Reasoning and Acting in Language Models. ICLR 2023."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2210.03629"
aliases:
  - Yao 2023
  - ReAct paper
tags:
  - agent-architecture
  - reasoning
related_ids:
  - concept-react
  - concept-reflexion
---

# Yao et al. — ReAct (ICLR 2023)

Yao et al. introduce ReAct, a prompting paradigm that interleaves language model reasoning traces with actions against external environments. The paper identifies a gap between two emerging capabilities: chain-of-thought reasoning (which improves multi-step problem solving but is prone to hallucination) and action-based tool use (which grounds the model in real information but lacks explicit reasoning). ReAct combines both by structuring the LLM's output as alternating Thought, Action, and Observation steps within a single prompt.

The Thought step is a natural language reasoning trace: the model articulates what it knows, what it needs, and what it plans to do. The Action step is a structured call to an external tool (Wikipedia search, environment command, etc.). The Observation step is the result returned by that tool. The model then continues to the next Thought, informed by the observation. This loop continues until the agent produces a final answer.

Evaluation covers two task categories: knowledge-intensive reasoning (HotpotQA, FEVER fact verification) using Wikipedia as the action space, and decision-making (ALFWorld household tasks, WebShop online shopping) using text game and web environments. ReAct outperforms chain-of-thought-only and acting-only baselines across all benchmarks.

## Key Findings

- On HotpotQA: ReAct outperforms chain-of-thought prompting by grounding multi-hop reasoning in retrieved Wikipedia evidence, reducing hallucinated fact chains
- On ALFWorld: +34% success rate over imitation learning / RL baselines; reasoning traces allow the agent to recover from failed actions by replanning
- On WebShop: +10% over RL baseline; the agent uses reasoning to interpret product attributes and filter candidates
- Combining ReAct with chain-of-thought (using CoT for reasoning traces) yields the best results, showing the two are complementary
- Human evaluators find ReAct's reasoning traces interpretable and aligned with ground-truth reasoning paths, improving trustworthiness

## Relevance to VaultMind

ReAct establishes the pattern that motivates VaultMind as an agent tool. A ReAct agent issues actions to external sources when it needs grounded information; VaultMind provides exactly such a source for user-specific knowledge. The `search`, `recall`, and [[context-pack|context-pack]] commands map directly to ReAct's action space — a ReAct agent can query VaultMind, receive structured observations (note content, graph relationships), and incorporate those observations into subsequent reasoning steps. This makes VaultMind a natural fit for any ReAct-based agent system that needs to access the user's personal knowledge base.
