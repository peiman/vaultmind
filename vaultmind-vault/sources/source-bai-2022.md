---
id: source-bai-2022
type: source
title: "Bai et al. Constitutional AI: Harmlessness from AI Feedback (2022)"
created: 2026-04-29
url: "https://arxiv.org/abs/2212.08073"
tags:
  - llms
  - deep-learning
  - alignment
  - rlaif
related_ids:
  - concept-constitutional-ai
  - concept-rlhf
---

# Bai et al. — Constitutional AI (2022)

Bai and the Anthropic alignment team introduced Constitutional AI, a method for training a harmless assistant without harmful-content human labels. The model is given a written "constitution" — a set of natural-language principles — and asked to critique and revise its own responses against those principles, producing self-generated training data. A reward model is then trained on AI-generated preference labels (RLAIF) instead of human ones.

The paper showed that a model trained with constitutional self-critique plus RLAIF can be both more helpful and more harmless than one trained with the standard human-feedback RLHF stack. The constitution-based approach is the lineage of Claude's training and is the canonical reference for principle-based alignment and RLAIF.
