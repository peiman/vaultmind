---
id: source-ouyang-2022
type: source
title: "Ouyang et al. Training Language Models to Follow Instructions with Human Feedback (InstructGPT, 2022)"
created: 2026-04-29
url: "https://arxiv.org/abs/2203.02155"
tags:
  - llms
  - deep-learning
  - alignment
  - rlhf
related_ids:
  - concept-rlhf
  - concept-instruction-tuning
---

# Ouyang et al. — InstructGPT (2022)

Ouyang and the OpenAI alignment team applied [[source-christiano-2017|Christiano 2017]]-style RLHF to GPT-3, producing InstructGPT. The training stack — supervised fine-tuning on human demonstrations, then reward-model training on human comparisons, then PPO against the reward model with a KL penalty against the SFT policy — is the canonical three-stage RLHF recipe that ChatGPT, Claude, Gemini, and most aligned chat models inherited.

The headline result was that the 1.3B InstructGPT was preferred to the 175B vanilla GPT-3 by human raters, demonstrating that alignment via human feedback can deliver more "value per parameter" than scale alone. The paper also documented honesty and toxicity improvements and the alignment tax (modest capability regressions on some benchmarks).
