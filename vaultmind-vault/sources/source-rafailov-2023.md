---
id: source-rafailov-2023
type: source
title: "Rafailov et al. Direct Preference Optimization: Your Language Model is Secretly a Reward Model (2023)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2305.18290"
tags:
  - llms
  - deep-learning
  - alignment
  - dpo
related_ids:
  - concept-dpo
  - concept-rlhf
---

# Rafailov et al. — Direct Preference Optimization (2023)

Rafailov, Sharma, Mitchell, Ermon, Manning, and Finn (Stanford) showed that the optimal policy of the standard RLHF objective (KL-regularized reward maximization) can be expressed in closed form as a function of the reward, and inverted: the reward model implied by any policy is determined by the policy itself. This lets one train directly on preference pairs using a simple classification-style loss, without ever fitting an explicit reward model or running PPO.

DPO matched or exceeded PPO-RLHF on summarization and dialogue benchmarks while being far simpler to implement and more stable to train. The paper opened the floodgates of preference-based fine-tuning methods (IPO, KTO, ORPO, SimPO) and made alignment training accessible to teams without an RL infrastructure.
