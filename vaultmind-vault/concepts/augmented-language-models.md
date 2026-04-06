---
id: concept-augmented-language-models
type: concept
title: Augmented Language Models
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - ALMs
  - Tool-Augmented LMs
tags:
  - llm-memory
  - survey
  - tool-use
related_ids:
  - concept-rag
  - concept-reflexion
  - concept-voyager
source_ids:
  - source-mialon-2023
---

## Overview

Augmented Language Models (Mialon et al., Meta AI Research, 2023) is a comprehensive survey that defines and taxonomizes language models enhanced with capabilities beyond next-token prediction. The survey (arXiv:2302.07842) categorizes augmentation along three orthogonal dimensions:

1. **Reasoning:** Decomposing complex tasks into subtasks — chain-of-thought, scratchpads, least-to-most prompting, program synthesis.
2. **Tool use:** Calling external systems — search engines, code interpreters, calculators, databases, APIs. The model decides when and how to invoke tools.
3. **Acting:** Taking actions with real-world effects — controlling browsers, writing files, executing shell commands, interacting with GUIs.

An ALM is defined as a language model that can do one or more of these. The survey covers roughly 100 papers published before early 2023, providing a map of the field at a moment of rapid expansion.

## Key Properties

- **Unified taxonomy:** Before this survey, reasoning, tool use, and acting were discussed in separate subcommunities. The ALM framing unifies them under a single capability model.
- **Memory as infrastructure:** External memory (retrieval, knowledge bases, vector stores) is treated as a prerequisite for effective tool use and acting — not a separate capability.
- **Controller/executor pattern:** Many ALM architectures separate a controller (the LLM deciding what to do) from executors (tools that carry out actions). This decomposition clarifies where memory fits.
- **Compositionality:** ALMs become more capable when augmentations compose — a model that can reason *and* use tools *and* act can accomplish tasks that none can handle alone.
- **Survey coverage:** Covers reasoning augmentation (ReAct, scratchpad, CoT), tool use (Toolformer, WebGPT, LaMDA), and acting (SayCan, Inner Monologue, code-generating agents).

## Connections

The ALM survey provides the conceptual vocabulary that situates VaultMind within the broader ecosystem. VaultMind is a **memory tool** in the ALM sense: an external system that an LLM can call to retrieve structured knowledge. This positions it alongside retrieval systems like [[rag|RAG]] rather than reasoning augmentations.

[[voyager|Voyager]] is an ALM that combines reasoning (iterative prompting), tool use (Minecraft API), and acting (in-game execution) with a persistent skill memory. [[reflexion|Reflexion]] is an ALM where the tool being used is the agent's own episodic memory buffer.

For VaultMind, the ALM framing clarifies the design contract: VaultMind is called by a controller LLM (via the `vaultmind` CLI) and returns structured context. The interface should be designed for machine consumption — fast, structured, token-efficient — not for human browsing. The expert panel (Hoffmann) noted this in Session 02, recommending that VaultMind's output formats optimize for LLM readability over human readability.
