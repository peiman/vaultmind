---
id: concept-reflexion
type: concept
title: Reflexion
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Reflexion Architecture
  - Self-Reflecting Agents
tags:
  - ai-memory
  - agent-architecture
  - episodic-memory
related_ids:
  - concept-generative-agents
  - concept-memgpt
source_ids:
  - source-shinn-2023
---

## Overview

Reflexion (Shinn et al., 2023) is an agent framework where the agent learns from trial-and-error by storing natural language reflections about past attempts in an episodic memory buffer. After a failed attempt, the agent generates a reflection ("I failed because I didn't check the edge case where...") and prepends it to future prompts.

Unlike weight-based learning (fine-tuning), Reflexion uses linguistic feedback stored in a memory buffer — making it interpretable, editable, and immediately effective without retraining.

## Key Properties

- **Episodic buffer:** Short natural language summaries of past attempts and outcomes
- **Self-evaluation:** Agent assesses its own performance after each attempt
- **Persistent across sessions:** Reflections carry forward to future runs
- **No gradient updates:** Learning happens through context, not weight changes
- **Diminishing returns:** Buffer eventually fills with redundant or conflicting reflections

## Connections

VaultMind has no built-in reflection mechanism. The expert panel (Session 02, Hoffmann) recommended a `reflection` note type with `source_ids` pointing to the notes that prompted the reflection. This would formalize the Reflexion pattern within VaultMind's type system — agents could write reflection notes via `note create --type reflection` and VaultMind could give them elevated traversal priority during [[context-pack|Context Pack]] assembly.

The key difference from [[generative-agents|Generative Agents]]' reflection: Generative Agents reflect on observations to produce insights. Reflexion reflects on actions to produce error corrections. Both are valuable for different agent tasks.
