---
id: concept-constitutional-ai
type: concept
title: Constitutional AI
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - CAI
  - RLAIF
  - Reinforcement Learning from AI Feedback
tags:
  - llms
  - deep-learning
  - alignment
  - rlaif
related_ids:
  - concept-rlhf
  - concept-dpo
  - concept-instruction-tuning
source_ids:
  - source-bai-2022
---

## Overview

Constitutional AI (CAI) is Anthropic's alignment method, introduced in [[source-bai-2022|Bai et al. 2022]], that trains a harmless assistant using a written "constitution" — a set of explicit natural-language principles — instead of relying on humans to label every harmful response. The model is asked to critique and revise its own outputs against the constitution, and the resulting revisions become training data. Preferences are then collected from an AI critic (RLAIF — Reinforcement Learning from AI Feedback) rather than human raters, scaling preference data far beyond what human annotation supports.

CAI is the lineage behind Claude's training. Its central claim is that a competent, instruction-tuned model can supervise its own alignment for harmlessness when given clear principles, freeing human annotation to focus on nuanced helpfulness and capability rather than catching toxic outputs.

## How It Works

The CAI training pipeline has two phases:

**1. Supervised constitutional revision (SL-CAI).**
- Sample a response from a helpful-only model to a prompt that might elicit harmful behavior.
- Ask the model to critique its own response against a constitutional principle (e.g., "Identify ways the response is harmful, unethical, or deceptive").
- Ask the model to rewrite the response addressing the critique.
- Fine-tune on (prompt → revised response) pairs.

The constitution comprises ~16 principles drawn from sources like the UN Declaration of Human Rights, Apple's terms of service, and lab-internal safety priorities. Different principles are sampled at each revision step.

**2. Reinforcement learning from AI feedback (RLAIF).**
- Sample pairs of responses from the SL-CAI model.
- An AI critic, prompted with a constitutional principle, picks which response is more aligned with it.
- Train a preference model on the AI-generated labels.
- Fine-tune the policy via PPO (or [[dpo|DPO]]) against the AI-trained preference model.

Mixing AI-generated harmlessness preferences with human-generated helpfulness preferences gives a model that is both helpful and harmless — the paper showed this Pareto-dominates pure-RLHF training on both axes.

## Recent Developments

- **Collective Constitutional AI (Anthropic, 2023)** — drafts a constitution via public deliberation rather than internal authorship.
- **Constitutional Classifiers (2024)** — separately trained guard models that screen inputs/outputs against constitutional principles at inference time.
- **Self-critique chains** — extended into multi-step debate and consultancy setups for scalable oversight research.
- **RLAIF generalization** — the AI-feedback paradigm has been adopted across the industry (Llama-Guard, Gemini's safety stack, OpenAI's deliberative alignment).

## Connections

CAI is a sibling of [[rlhf|RLHF]]; the only structural difference is who labels the preferences. The same Bradley-Terry reward modeling, KL-regularized policy optimization, and SFT bootstrap apply. CAI composes with [[dpo|DPO]] — the AI-labeled preference pairs feed any preference-optimization method.

The conceptual move — using model-generated supervision to train the next iteration of the model — is the same engine driving inference-time-compute scaling (o1/o3 use process reward models trained on AI-judged reasoning steps) and self-rewarding LMs. CAI was an early demonstration that scalable AI oversight of AI is tractable for at least narrow safety goals.

The constitution itself is the alignment artifact most amenable to public scrutiny: unlike opaque human-rater instructions, it is a written document that can be debated, revised, and audited.
