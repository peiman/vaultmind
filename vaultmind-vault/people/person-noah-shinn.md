---
id: person-noah-shinn
type: person
title: "Noah Shinn"
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Shinn
tags:
  - ai
  - llm-agents
  - self-reflection
related_ids:
  - concept-reflexion
  - source-shinn-2023
url: "https://noahshinn.com/"
---

## About

Noah Shinn is the lead author of Reflexion (2023), a framework in which LLM agents improve their performance on sequential decision tasks by generating natural-language self-critiques after each failed attempt and persisting those critiques as an episodic memory that conditions future trials — without any gradient-based weight updates.

## Key Contributions

Shinn's [[reflexion|Reflexion]] framework demonstrates that verbal self-reflection stored as memory is a viable substitute for fine-tuning: agents that can write and later read their own post-mortem analyses converge on better strategies over time. For VaultMind, this is the strongest argument for a dedicated `decision` note type that captures not just what was decided but why alternatives were rejected. An agent operating over a VaultMind vault with rich decision notes can avoid re-litigating past mistakes — the same mechanism Reflexion exploits, but organized as a curated knowledge base rather than a rolling in-context buffer.
