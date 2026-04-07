---
id: concept-react
type: concept
title: ReAct
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Reasoning and Acting
  - ReAct Framework
tags:
  - agent-architecture
  - reasoning
  - tool-use
related_ids:
  - concept-reflexion
  - concept-voyager
  - concept-augmented-language-models
source_ids:
  - source-yao-2023
---

## Overview

ReAct (Yao et al., ICLR 2023) proposes interleaving reasoning traces and actions within a single LLM prompt, allowing an agent to think through a problem and act on the world in alternating steps. Prior work treated reasoning (chain-of-thought) and acting (tool use, environment interaction) as separate paradigms; ReAct unifies them.

The structure is a loop: the agent produces a **Thought** (natural language reasoning about the current state, plan, or exception), then an **Action** (a call to an external tool or environment), then receives an **Observation** (the result), and repeats. Reasoning traces help the agent track its plan, handle unexpected results, and decide when to change strategy. Actions ground the reasoning in real information, preventing reasoning chains from drifting into hallucination.

On HotpotQA (multi-hop QA requiring evidence synthesis), ReAct outperforms chain-of-thought by using Wikipedia API actions to verify and retrieve facts rather than hallucinating them. On ALFWorld (text-based household tasks), ReAct achieves +34% over RL baselines. On WebShop (online shopping agent), +10% over RL baselines. arXiv:2210.03629.

## Key Properties

- **Interleaved thought-action-observation:** The agent alternates between natural language reasoning and external actions, with each cycle informed by the previous.
- **Hallucination reduction:** Grounding reasoning in actual retrieved evidence (Wikipedia API, environment observations) prevents the compounding errors seen in pure chain-of-thought.
- **Exception handling:** Explicit reasoning traces allow the agent to recognize when an action failed or returned unexpected results and adjust its plan accordingly.
- **Flexible action space:** Actions can target any external interface — knowledge bases, search APIs, code executors, web browsers, or structured tools like a CLI.
- **HotpotQA:** ReAct outperforms chain-of-thought, improving accuracy by reducing factual hallucination via Wikipedia lookups.
- **ALFWorld/WebShop:** +34% and +10% respectively over reinforcement learning baselines, demonstrating that natural language reasoning combined with acting outperforms reward-signal-only approaches.

## Connections

ReAct is a foundational agent architecture that [[reflexion|Reflexion]] builds on: Reflexion adds an explicit self-evaluation and memory-writing step after each episode, whereas ReAct provides only within-episode reasoning.

[[voyager|Voyager]] applies a similar thought-action-observation structure in a Minecraft environment with a skill library, showing that the ReAct pattern generalizes to open-ended continuous tasks.

[[augmented-language-models|Augmented Language Models]] provides a broader taxonomic framing that includes ReAct as one instance of tool-augmented reasoning.

For VaultMind, ReAct agents need external knowledge sources. VaultMind's `search` and `recall` commands are exactly the "Act" targets for a ReAct agent's knowledge-seeking actions. A ReAct agent working on a long-horizon task could issue `vaultmind search "authentication decision"` as an action, receive retrieved notes as an observation, reason about relevance, and continue — using the vault as a grounded external knowledge base rather than relying on parametric memory alone.
