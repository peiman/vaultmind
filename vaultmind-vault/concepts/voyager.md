---
id: concept-voyager
type: concept
title: Voyager
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Voyager Agent
  - LLM Lifelong Learning Agent
tags:
  - llm-memory
  - agent-architecture
  - skill-library
related_ids:
  - concept-generative-agents
  - concept-reflexion
  - concept-memgpt
source_ids:
  - source-wang-2023-voyager
---

## Overview

Voyager (Wang et al., 2023) is the first LLM-powered embodied lifelong learning agent, built in the Minecraft environment. Its key innovation is a **skill library**: a persistent, growing store of executable code (JavaScript functions) that the agent writes, verifies, and retrieves to accomplish progressively complex tasks. Skills are compositional — later skills call earlier ones — allowing the agent to build capability without catastrophic forgetting.

The architecture has three tightly integrated components:

1. **Automatic curriculum:** A GPT-4 module proposes the next task based on the agent's current inventory and skill level, always pushing toward the frontier of achievable difficulty.
2. **Skill library:** Each successful task generates a JavaScript function stored with a natural language description. At task time, the library is queried by embedding similarity to retrieve relevant skills.
3. **Iterative prompting with self-verification:** The agent attempts a task, receives environment feedback (errors, observations), and refines its code in a self-correcting loop before committing the skill to the library.

Result: Voyager discovers 3.3x more unique items, travels 2.3x further, and unlocks more tech tree milestones than prior SOTA methods including AutoGPT and ReAct-based agents.

## Key Properties

- **Code as memory:** Skills are stored as executable code, not natural language summaries. This makes retrieval precise and execution reliable — retrieved skills run deterministically.
- **No catastrophic forgetting:** The skill library grows monotonically. Old skills are never overwritten; the curriculum ensures new skills build on rather than replace existing ones.
- **Embedding-based skill retrieval:** Natural language task descriptions are embedded; the top-5 most relevant skills are retrieved and provided as context for the current task.
- **Self-verification loop:** Before a skill is stored, the agent verifies success via environment state checks, reducing the rate of storing faulty skills.
- **GPT-4 as the core reasoner:** The curriculum, skill generation, and self-verification all run through GPT-4. The LLM is the planner; code execution is the memory.

## Connections

Voyager's skill library is a form of **procedural memory** — knowledge encoded as executable procedures rather than declarative facts. This contrasts with [[generative-agents|Generative Agents]]' episodic memory stream (natural language observations) and [[reflexion|Reflexion]]'s episodic reflection buffer (natural language error corrections).

The skill retrieval mechanism (embedding similarity over natural language descriptions) resembles [[rag|RAG]] but operates over code rather than documents. The stored artifact is executable, not just informational — a meaningful distinction for agent reliability.

For VaultMind, Voyager suggests a potential `skill` note type: structured notes containing reusable agent procedures, retrieved by embedding similarity and executable by the consuming agent. VaultMind's typed graph could track which skills were used to accomplish which goals, enabling richer curriculum planning than Voyager's inventory-based heuristic.
