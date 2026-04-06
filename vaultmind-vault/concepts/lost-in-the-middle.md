---
id: concept-lost-in-the-middle
type: concept
title: Lost in the Middle
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Position Bias in LLMs
  - U-shaped Attention
tags:
  - long-context
  - evaluation
  - retrieval
related_ids:
  - concept-working-memory
  - concept-context-pack
  - concept-rag
source_ids:
  - source-liu-2023-lost
---

## Overview

Liu et al. (Stanford, 2023; published TACL 2024) demonstrated that LLMs do not use long contexts uniformly. Performance on multi-document question answering degrades significantly when the relevant information is positioned in the middle of the context window, even when models claim to support long contexts.

The pattern is U-shaped: models perform best when relevant documents are at the beginning or end of the context, and worst in the middle. This holds across model families and scales. Even models with explicitly extended context windows (16K+) exhibit the same positional bias. The effect was replicated across both multi-document QA tasks and synthetic key-value retrieval tasks, ruling out dataset-specific artifacts.

## Key Properties

- **U-shaped performance curve:** Best at position 0 (beginning) and position N (end); worst in the middle
- **Model-agnostic:** Observed across GPT-3.5, GPT-4, Claude, and open-source models
- **Scale-independent:** Longer context windows do not eliminate the bias; they may widen the middle-degraded zone
- **Task-general:** Affects both open-domain QA and controlled key-value retrieval
- **Practical implication:** Document ordering is a retrieval engineering concern, not just a retrieval quality concern

## Connections

This finding has a direct design implication for VaultMind's [[context-pack|Context Pack]] priority ordering. The current packing algorithm orders by edge confidence tier (explicit_relation > explicit_link > inferred), which — as a side effect — places the most relevant content first. This is empirically correct: the most relevant neighbors should occupy the beginning of the context payload, not the middle.

The practical recommendation is to reinforce this principle explicitly: when assembling a context pack, the highest-confidence neighbors should anchor the beginning and end of the payload, with medium-confidence content in the middle. This mirrors the "Lost in the Middle" U-shape — the LLM will attend most to the edges of whatever context VaultMind provides.

This also motivates VaultMind's `budget_exhausted` truncation signal: rather than padding with low-confidence material to fill the budget, it is better to stop and signal exhaustion. Padding the middle with weak evidence is actively harmful under this finding.

The result informs how [[rag|RAG]] systems should present multi-document retrievals generally: top-ranked documents should be placed at the beginning and end of the prompt, not concatenated in rank order which leaves the most-relevant document buried in the middle.
