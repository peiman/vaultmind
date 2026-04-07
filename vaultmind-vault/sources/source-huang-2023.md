---
id: source-huang-2023
type: source
title: "Huang, L., et al. (2023). A Survey on Hallucination in Large Language Models: Principles, Taxonomy, Challenges, and Open Questions. arXiv:2311.05232."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2311.05232"
aliases:
  - Huang 2023
  - Hallucination Survey
tags:
  - llm-memory
  - evaluation
related_ids:
  - concept-hallucination-grounding
  - concept-rag
---

# Huang et al. — Hallucination Survey (2023)

Huang and colleagues present a systematic survey of hallucination in large language models, providing a unified taxonomy, analysis of causes, and review of detection and mitigation strategies. The survey defines hallucination as the generation of content that is fluent and confident but unsupported by or contradictory to verifiable knowledge, and distinguishes between factuality hallucination (contradiction of world facts) and faithfulness hallucination (contradiction of provided context).

The paper covers hallucination manifestations across a wide range of NLP tasks: open-domain question answering, abstractive summarization, dialogue generation, machine translation, and knowledge-grounded generation. For each task type, the authors describe how hallucination manifests differently and what evaluation protocols are applicable. The survey also reviews the principal causes of hallucination, including data noise in pretraining corpora, knowledge gaps, generation decoding strategies, and exposure bias from teacher forcing.

## Key Findings

- Hallucination is taxonomized into factuality hallucination (contradicts verifiable world knowledge) and faithfulness hallucination (contradicts or is unsupported by the provided context); these have different causes and require different mitigations
- Retrieval-augmented generation is identified as a primary architectural mitigation for factuality hallucination, reducing but not eliminating the phenomenon — faithfulness hallucination persists even with accurate retrieved context
- Detection approaches surveyed include reference-based metrics, model-based entailment checkers, and prompting-based self-consistency methods; no single method generalizes reliably across task types
- Mitigation strategies surveyed include: improved pretraining data curation, instruction tuning on factual tasks, RLHF with factuality feedback, retrieval augmentation, and chain-of-thought prompting
- The survey identifies source attribution and citation generation as an underexplored mitigation direction: grounding generated claims in specific retrieved passages enables downstream verification even when the generation model is imperfect

## Relevance to VaultMind

This survey provides the conceptual foundation for [[hallucination-grounding|VaultMind's grounding value proposition]]. The distinction between factuality and faithfulness hallucination is directly relevant to VaultMind's design: the vault retrieval system addresses factuality hallucination by providing domain-specific, personal, and time-sensitive knowledge the model cannot have parametrically; faithfulness hallucination requires additional mitigations at the generation layer (e.g., prompting the agent to cite sources, or implementing a faithfulness check over the agent's response against retrieved vault content). The survey's identification of source attribution as an underexplored mitigation aligns with VaultMind's source_ids and provenance tracking design.
