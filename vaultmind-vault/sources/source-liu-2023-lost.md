---
id: source-liu-2023-lost
type: source
title: "Liu, N. F., et al. (2023). Lost in the Middle: How Language Models Use Long Contexts. TACL 2024, 12, 157-173."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2307.03172"
aliases:
  - Liu 2023 Lost in the Middle
  - Lost in the Middle paper
tags:
  - long-context
  - evaluation
related_ids:
  - concept-lost-in-the-middle
  - concept-working-memory
  - concept-context-pack
---

# Liu et al. — Lost in the Middle (2023/2024)

Liu et al. (Stanford NLP) conducted a systematic empirical study of how LLMs use information distributed across long contexts. The central finding: LLMs do not attend to context uniformly. Performance on multi-document question answering follows a U-shaped curve with respect to the position of the relevant document — highest when the relevant document is first or last in the context, lowest when it is in the middle. The paper was posted to arXiv in July 2023 and published in Transactions of the Association for Computational Linguistics (TACL) in 2024 (vol. 12, pp. 157–173).

## Key Findings

- **U-shaped performance curve:** Relevant information at the beginning or end of context yields the best accuracy; middle position yields the worst — a consistent pattern across model families
- **Model-agnostic:** The effect was observed across GPT-3.5-Turbo, GPT-4, Claude-1, and several open-source models, ruling out architecture-specific explanations
- **Extended context windows do not help:** Models with 16K context windows showed the same positional bias as models with shorter windows; more context capacity does not improve middle-position utilization
- **Replicated on controlled tasks:** Key-value retrieval tasks (a synthetic benchmark with no ambiguity) showed the same pattern, confirming it is a fundamental attention characteristic rather than a dataset artifact

## Relevance to VaultMind

The Lost in the Middle finding is a direct design constraint for VaultMind's [[context-pack|Context Pack]] assembly. The priority ordering — explicit_relation first, explicit_link second, inferred last — already places the most relevant neighbors at the start of the payload by construction. This paper provides empirical justification for that ordering: the LLM consuming the context pack will attend most to whatever VaultMind places at the beginning.

The result also reinforces VaultMind's conservative truncation policy: when the token budget is exhausted, it is better to emit `budget_exhausted: true` and omit low-confidence neighbors than to pad the middle of the context with weakly-relevant material. Weak evidence in the middle is not just neutral — it may actively displace the model's attention from the high-confidence material at the edges.
