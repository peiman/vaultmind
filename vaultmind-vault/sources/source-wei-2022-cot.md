---
id: source-wei-2022-cot
type: source
title: "Wei et al. Chain-of-Thought Prompting Elicits Reasoning in Large Language Models (2022)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2201.11903"
tags:
  - llms
  - deep-learning
  - prompting
  - reasoning
related_ids:
  - concept-chain-of-thought
---

# Wei et al. — Chain-of-Thought Prompting (2022)

Wei and colleagues at Google Research demonstrated that few-shot prompting LLMs with worked-out intermediate reasoning steps (a "chain of thought") substantially improves performance on arithmetic, commonsense, and symbolic reasoning tasks. The effect was qualitatively new at sufficient scale: CoT prompting produced negligible gains on small models and dramatic gains (often 20+ points absolute) starting around 100B parameters, qualifying it as an emergent capability.

The paper defined a research program on prompt-engineered reasoning, motivated zero-shot CoT ("Let's think step by step"), and is the lineage point for [[concept-tree-of-thought|Tree-of-Thoughts]], self-consistency, and the broader inference-time-compute paradigm.
