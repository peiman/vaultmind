---
id: source-shinn-2023
type: source
title: "Shinn et al. Reflexion: Language Agents with Verbal Reinforcement Learning (2023)"
created: 2026-04-03
vm_updated: 2026-04-03
url: "https://arxiv.org/abs/2303.11366"
aliases:
  - Shinn 2023
  - Reflexion paper
tags:
  - ai-memory
  - agent-architecture
related_ids:
  - concept-reflexion
---

# Shinn et al. — Reflexion (2023)

Reflexion replaces gradient-based reinforcement learning with verbal feedback stored in an episodic memory buffer. After each task attempt the agent produces a natural-language self-evaluation of what went wrong and why; this critique is prepended to future attempts as persistent context. Over multiple trials the agent accumulates a layered record of its own failure modes and workarounds, effectively learning without weight updates.

The central insight is that language itself can serve as the update signal, making improvement interpretable and reversible in a way gradient descent is not. The episodic memory buffer plays the role of a personal error log—a structured record that shapes future reasoning without overwriting prior knowledge. This connects directly to [[reflexion|Reflexion]] as a vault concept and offers a template for how agents should handle correction and revision over time.

VaultMind uses a Reflexion-inspired feedback loop when a user corrects or overwrites a generated suggestion. The correction is logged as a meta-note and surfaced in subsequent generation passes, allowing the system to adapt its recommendations without retraining. This makes VaultMind's behavior over a vault auditable: every behavioral shift traces back to a human-authored correction note.
