---
id: concept-chain-of-thought
type: concept
title: Chain-of-Thought Prompting
created: 2026-04-29
aliases:
  - CoT
  - Chain of Thought
tags:
  - llms
  - deep-learning
  - prompting
  - reasoning
related_ids:
  - concept-tree-of-thought
  - concept-react
  - concept-reflexion
  - concept-scaling-laws
source_ids:
  - source-wei-2022-cot
---

## Overview

Chain-of-Thought (CoT) prompting is a technique for eliciting better reasoning from LLMs by asking them to generate intermediate reasoning steps before producing a final answer. Introduced by [[source-wei-2022-cot|Wei et al. 2022]], it improves performance on arithmetic, commonsense, and symbolic reasoning tasks dramatically — often 20+ points absolute — without any change to the underlying model.

The simplest form is few-shot CoT: include examples in the prompt where each demonstration shows step-by-step reasoning leading to the answer. The model then continues the pattern on the new question. Zero-shot CoT (Kojima et al. 2022) showed that the prompt "Let's think step by step" alone, with no exemplars, suffices to elicit similar reasoning in sufficiently capable models.

CoT was the first widely-discussed example of an emergent capability: small models showed no benefit from CoT prompting and sometimes did worse, while large models showed sharp gains. This emergence — capabilities appearing only above a scale threshold — became one of the central empirical observations driving the post-2022 scaling agenda.

## Key Mechanism

Why does intermediate-step generation help? Several explanations, mostly compatible:

- **Effective compute per answer.** Each generated token gets a forward pass through the model. CoT lets the model spend many forward passes on intermediate state before committing to an answer, effectively expanding the depth of computation per question. Without CoT, the answer must be computed in a single pass.
- **Decomposition.** Hard problems decompose into easier sub-problems. CoT exposes that decomposition explicitly, so each sub-step is within the model's per-token capability even when the joint problem isn't.
- **In-distribution priming.** Pretraining text contains worked-out reasoning (textbooks, math solutions, code traces). CoT prompts the model to enter this generative regime rather than the "produce a confident terse answer" regime.
- **Self-conditioning.** Each generated step becomes context for the next, so the model accumulates intermediate facts it would otherwise have to re-derive.

## Recent Developments

- **Self-consistency (Wang et al. 2022)** — sample many CoT traces, then majority-vote the final answers. A simple addition that consistently improves CoT.
- **[[tree-of-thought|Tree-of-Thoughts (Yao et al. 2023)]]** — generalize the linear chain to a search tree with proposal, self-evaluation, and backtracking.
- **Program-aided reasoning (PAL, PoT)** — generate a Python program that computes the answer rather than executing math in natural language.
- **[[react|ReAct]]** — interleave reasoning steps with tool calls (search, calculator) to ground intermediate state externally.
- **Process reward models** — supervised at the step level, used to train models (o1, o3, DeepSeek-R1) where the chain of thought is internalized and refined via RL.
- **Chain-of-thought faithfulness** — research showing that the verbalized chain doesn't always reflect the actual computation. The model's stated reasoning may diverge from what's load-bearing in its hidden activations.

## Connections

CoT sits at the root of the modern inference-time-compute paradigm. Reasoning models (o1, o3, R1, Gemini Thinking) are essentially CoT scaled up: instead of generating one short chain at inference, they generate very long internal chains, optimized via [[rlhf|RL]] against a process reward model. The progression CoT → self-consistency → [[tree-of-thought|ToT]] → search-augmented reasoning → reasoning-trained models is one line of the field's history.

The cognitive analog is verbal mediation in problem-solving — children solve hard arithmetic by saying steps out loud, and adults who suppress sub-vocalization perform worse on novel multi-step problems. CoT is the LLM equivalent of "show your work."
