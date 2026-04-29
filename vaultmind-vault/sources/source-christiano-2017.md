---
id: source-christiano-2017
type: source
title: "Christiano et al. Deep Reinforcement Learning from Human Preferences (2017)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/1706.03741"
tags:
  - llms
  - deep-learning
  - alignment
  - rlhf
related_ids:
  - concept-rlhf
---

# Christiano et al. — Deep RL from Human Preferences (2017)

Christiano, Leike, Brown, Martic, Legg, and Amodei introduced the modern RLHF formulation: train a reward model by asking humans to compare pairs of agent trajectories and predict the preferred one, then use that reward model as the reward signal for a deep RL algorithm. The paper demonstrated the technique on Atari games and simulated robotics tasks, learning behaviors that would have required hand-crafted reward functions otherwise.

This is the foundational paper that the InstructGPT/ChatGPT alignment stack rests on. Three contributions matter most: (1) reward modeling from pairwise preferences (Bradley-Terry), (2) the demonstration that comparatively few human comparisons (~1000s) suffice when used as supervision for a learned reward model, and (3) the asynchronous-elicitation pattern where preference data and policy training proceed concurrently.
