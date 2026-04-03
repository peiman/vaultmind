---
id: concept-working-memory
type: concept
title: Working Memory
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Short-Term Memory
  - Active Memory
  - Context Window
tags:
  - cognitive-science
  - ai-memory
  - memory-systems
related_ids:
  - concept-context-pack
  - concept-memgpt
  - concept-act-r
source_ids: []
---

## Overview

Working memory is the cognitive system responsible for temporarily holding and manipulating information during active reasoning. In human cognition, it has limited capacity (~4 chunks, per Cowan 2001) and requires active maintenance — information decays rapidly without rehearsal.

In LLM agents, working memory maps to the context window — the tokens currently visible to the model during inference. Like human working memory, it has hard capacity limits (context length) and everything outside it is effectively invisible.

## Key Properties

- **Limited capacity:** ~4 chunks (human) / N tokens (LLM context window)
- **Active maintenance required:** Information must be refreshed or it's lost
- **Gateway to long-term memory:** Items in working memory can be encoded into long-term storage
- **Executive control:** Decides what to attend to, what to retrieve, what to discard

## Connections

VaultMind serves as long-term archival memory — it does not manage what's currently in an agent's context window. The [[Context Pack]] mechanism bridges archival → working memory by assembling token-budgeted payloads, but there is no feedback loop: VaultMind doesn't track what the agent has already loaded or what it's currently reasoning about.

The [[MemGPT]] architecture explicitly models this tier boundary with a memory manager that moves information between working memory (context window) and archival storage. VaultMind could serve as MemGPT's archival backend.
