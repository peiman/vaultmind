---
id: concept-hallucination-grounding
type: concept
title: Hallucination and Grounding
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - LLM Hallucination
  - Factual Grounding
tags:
  - llm-memory
  - evaluation
  - factuality
related_ids:
  - concept-rag
  - concept-self-rag
  - concept-corrective-rag
source_ids:
  - source-huang-2023
---

## Overview

Hallucination in large language models refers to the generation of content that is fluent, plausible-sounding, and confidently stated, but factually incorrect or unsupported by the available evidence. Huang et al. (2023) provide a comprehensive taxonomy distinguishing two primary hallucination types: factuality hallucination, where generated content contradicts verifiable world facts; and faithfulness hallucination, where generated content is internally inconsistent or contradicts the context provided to the model.

Hallucination arises from the fundamental architecture of autoregressive language models: each token is predicted from the distribution learned during training, and that distribution reflects statistical co-occurrence patterns in training data rather than a verified factual world model. When the model encounters a prompt about a topic where its training data is sparse, contradictory, or outdated, it may generate statistically plausible but factually incorrect continuations. The model has no mechanism for distinguishing between what it "knows" reliably and what it is confabulating.

**Factuality hallucination** includes: generating incorrect names, dates, statistics, or entities (intrinsic factuality errors), and fabricating citations, quotes, events, or entities that do not exist (extrinsic or fabrication hallucinations). These are independent of any provided context and reflect failures in the model's parametric knowledge.

**Faithfulness hallucination** occurs when the model contradicts or ignores information explicitly provided in its context (e.g., retrieved passages, user-provided documents, conversation history). The model may summarize a document incorrectly, misattribute statements, or insert claims that are absent from the provided material. This type is particularly relevant for RAG systems, where the model is expected to remain faithful to retrieved evidence.

**Grounding** is the practice of anchoring model outputs in specific, verifiable evidence sources. A grounded response cites or directly quotes the source material, allowing the reader to verify the claim. RAG inherently promotes grounding by providing relevant context at generation time, but does not guarantee it — models may still ignore or contradict retrieved passages.

## Key Properties

- **Hallucination vs. error:** Hallucination specifically refers to confident generation of incorrect content, not all model errors. A model that says "I'm not sure" is not hallucinating; a model that confidently invents a fictional citation is.
- **RAG reduces but does not eliminate hallucination:** By providing retrieved evidence, RAG gives the model a factual anchor for its generation. Empirically, RAG substantially reduces factuality hallucination on knowledge-intensive tasks. However, faithfulness hallucination persists: models can ignore provided context, selectively attend to only part of it, or generate claims that blend retrieved and parametric knowledge in incorrect ways.
- **Retrieval quality and hallucination interact:** If the retrieved passages are themselves low quality, irrelevant, or contradictory, the model may hallucinate to reconcile conflicts or fill gaps. This is why retrieval quality is directly linked to output faithfulness. [[corrective-rag|Corrective RAG]] explicitly addresses this by detecting low-quality retrieved passages and triggering web search fallback.
- **Self-RAG's critique tokens:** [[self-rag|Self-RAG]] addresses faithfulness hallucination by training the model to generate special tokens that assess whether its own output is supported by the retrieved passage, directly operationalizing faithfulness as a training objective.
- **Source provenance as a mitigation:** Returning source citations alongside generated answers allows downstream verification. This does not prevent hallucination but enables detection and correction by the user or a verification layer.
- **Huang et al. 2023 survey scope:** The survey covers hallucination across multiple NLP tasks (summarization, dialogue, QA, machine translation, knowledge-grounded generation) and organizes causes, detection methods, and mitigation strategies into a unified taxonomy. It is a key reference for understanding the full landscape of the problem.

## Connections

[[rag|RAG]] is the primary architectural mitigation for factuality hallucination — providing retrieved evidence reduces reliance on potentially incorrect parametric knowledge.

[[self-rag|Self-RAG]] and [[corrective-rag|Corrective RAG]] extend standard RAG to also address faithfulness hallucination by incorporating quality assessment of both retrieved passages and generated outputs.

For VaultMind, hallucination grounding is the foundational value proposition. An agent operating without vault access must rely on parametric memory, which is subject to factuality hallucination for domain-specific, time-sensitive, or personal knowledge. VaultMind provides the agent with factual vault content at query time, grounding generation in the user's own notes rather than the model's approximation of that knowledge. Source provenance tracking — recording which vault notes contributed to a response via source_ids and confidence levels — enables the agent to cite its vault sources, providing the user with an auditable chain from claim back to original note. This directly operationalizes grounding at the application layer, even without changes to the underlying model.
