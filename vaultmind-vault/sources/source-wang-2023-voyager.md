---
id: source-wang-2023-voyager
type: source
title: "Wang, G., et al. (2023). Voyager: An Open-Ended Embodied Agent with Large Language Models. arXiv:2305.16291."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2305.16291"
aliases:
  - Wang 2023 Voyager
  - Voyager paper
tags:
  - llm-memory
  - agent-architecture
related_ids:
  - concept-voyager
  - concept-generative-agents
---

# Wang et al. — Voyager (2023)

Wang et al. introduced Voyager, the first LLM-powered embodied lifelong learning agent, evaluated in the open-ended Minecraft environment. The agent uses GPT-4 for all reasoning tasks and achieves continuous skill acquisition without catastrophic forgetting through a persistent, growing skill library.

The three-component architecture is tightly interdependent: the automatic curriculum proposes tasks that require new skills, iterative prompting with self-verification produces verified executable code, and the skill library stores verified functions for future retrieval. Without any single component, performance degrades significantly — ablations show that removing the skill library causes the most severe drop.

Key empirical results: Voyager discovers 3.3x more unique items than AutoGPT, travels 2.3x further, and unlocks the full Minecraft tech tree faster than all baselines. The improvement is attributed primarily to the skill library enabling compositional capability growth — later skills call earlier skills, building a hierarchy analogous to a software library.

The skill retrieval mechanism uses OpenAI's text-embedding-ada-002 to embed natural language task descriptions, then retrieves the top-5 most similar skill descriptions from the library. This embedding-based retrieval is pragmatic but coarse; the authors note that more sophisticated retrieval (e.g., structured queries over skill metadata) could improve precision.

For VaultMind, Voyager establishes the viability of code-as-memory: storing executable procedures as the primary memory artifact, rather than natural language summaries. The insight that verification before storage dramatically improves retrieval quality is directly applicable to VaultMind note quality control.
