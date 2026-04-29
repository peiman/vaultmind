---
id: concept-rlhf
type: concept
title: RLHF (Reinforcement Learning from Human Feedback)
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Reinforcement Learning from Human Feedback
  - Preference Learning
tags:
  - llms
  - deep-learning
  - alignment
  - rlhf
related_ids:
  - concept-dpo
  - concept-constitutional-ai
  - concept-instruction-tuning
  - concept-gpt
source_ids:
  - source-christiano-2017
  - source-ouyang-2022
---

## Overview

Reinforcement Learning from Human Feedback (RLHF) is a training pipeline that aligns a language model's behavior with human preferences without requiring a hand-crafted reward function. Humans compare pairs of model outputs and label which they prefer; a reward model is trained to predict those preferences; then the language model is fine-tuned via reinforcement learning to maximize the reward model's score, regularized against drifting too far from its starting policy.

RLHF is the technique that turned raw pretrained LLMs (GPT-3, PaLM, LLaMA-base) into the helpful, mostly-harmless assistants users now interact with. ChatGPT, Claude, Gemini, and GPT-4 all rely on it as the central alignment step.

## How It Works

The canonical three-stage [[source-ouyang-2022|InstructGPT]] recipe:

1. **Supervised fine-tuning (SFT).** Start from a pretrained base model. Fine-tune on a curated dataset of human-written demonstrations — high-quality examples of how the model should respond. This produces a competent instruction-follower (related to [[concept-instruction-tuning|instruction tuning]]) but doesn't yet reflect preferences over alternative responses.

2. **Reward modeling (RM).** Sample multiple completions from the SFT model for the same prompt. Have humans rank them. Train a reward model — typically the SFT model with a scalar head — under the Bradley-Terry preference model: given a chosen response y_w and a rejected response y_l, maximize log σ(r(y_w) − r(y_l)).

3. **RL fine-tuning.** Optimize the policy π against the reward model using PPO (Proximal Policy Optimization), with a per-token KL penalty against the SFT reference policy:
   
   max_π E[r(y) − β · KL(π ‖ π_SFT)]
   
   The KL term prevents reward hacking and mode collapse — the policy stays "close to natural language" while improving on preferred axes.

[[source-christiano-2017|Christiano et al. 2017]] established the foundation in deep RL on Atari and robotics; Stiennon et al. 2020 (summarization) and [[source-ouyang-2022|Ouyang et al. 2022]] (InstructGPT) brought it to language.

## Recent Developments

- **[[concept-dpo|DPO]] (2023)** showed that the RL stage can be replaced with a simple classification loss derived from the same KL-regularized objective — no reward model, no PPO.
- **[[concept-constitutional-ai|Constitutional AI / RLAIF]] (2022)** replaces human preference labels with AI-generated ones, scaling preference data dramatically.
- **Process reward models** score reasoning steps individually, enabling RLHF to target reasoning quality, not just final answers — a key ingredient in o1/o3-style training.
- **Online RLHF** continually collects new preferences from deployed model interactions, closing the loop between user preferences and model behavior.
- **Iterated DPO / SPIN / self-rewarding** mix preference learning with model-generated comparisons.

## Connections

RLHF stands on top of [[concept-instruction-tuning|instruction tuning]] (SFT is itself instruction tuning) and is deeply entangled with [[concept-scaling-laws|scaling]] — larger reward models track preferences more reliably, and larger policies have more headroom for the RL step to find improvement directions.

The major successors and rivals: [[concept-dpo|DPO]] (simpler optimization), [[concept-constitutional-ai|Constitutional AI]] (AI-generated feedback, principle-based). All three coexist in the modern alignment stack — Anthropic combines all of them.

The cognitive analog: RLHF is operant conditioning at the policy level, with the reward model as a learned model of the reinforcer. The KL penalty is the regularizer that stops "Goodhart's law" — overoptimizing the proxy at the expense of what was actually wanted.
