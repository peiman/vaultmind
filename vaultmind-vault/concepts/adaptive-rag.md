---
id: concept-adaptive-rag
type: concept
title: Adaptive RAG
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Adaptive Retrieval
  - RAG Routing
tags:
  - retrieval
  - rag-variant
  - routing
related_ids:
  - concept-rag
  - concept-self-rag
  - concept-corrective-rag
source_ids:
  - source-jeong-2024
---

## Overview

Adaptive RAG (Jeong et al., NAACL 2024) addresses a fundamental inefficiency in standard RAG systems: applying the same retrieval strategy to every query, regardless of whether retrieval is actually needed or whether a single retrieval step is sufficient. The paper shows that a trained classifier can reliably predict query complexity and route queries through the appropriate strategy: simple queries that the model can answer from parametric knowledge skip retrieval entirely; moderately complex queries go through single-step RAG; highly complex or multi-faceted queries trigger multi-step iterative retrieval.

The classifier is trained on (query, complexity-label) pairs derived from the downstream QA performance of each strategy — if a query can be answered correctly without retrieval, it is labeled as simple; if it requires multiple retrieval steps to answer correctly, it is labeled as complex. The classifier is a small model (a fine-tuned T5 or similar) that adds minimal latency compared to the retrieval cost it saves. At inference time, the classifier's routing decision determines which pipeline branch the query enters.

This routing framework is distinct from [[self-rag|Self-RAG]]'s per-token retrieval decisions and [[corrective-rag|Corrective RAG]]'s quality assessment loop. Adaptive RAG makes a single up-front routing decision before any generation occurs, rather than checking quality during or after generation. The result is a system that is faster and cheaper for simple queries while still performing well on complex ones.

## Key Properties

- **Three routing tiers:** No-retrieval (parametric answer only), single-step RAG (retrieve once, generate), and multi-step RAG (iterative retrieval and reasoning). Each tier is optimized for a different region of the query complexity spectrum.
- **Trained complexity classifier:** The routing decision is made by a small trained classifier, not a heuristic or a large model prompted to self-assess. This keeps routing latency low and makes the routing decision predictable.
- **Complexity derived from QA outcomes:** Training labels are derived empirically from whether each query strategy succeeds on the downstream QA task, grounding complexity in actual retrieval utility rather than surface query features.
- **Avoids over-retrieval:** Standard RAG always retrieves, even for queries the model could answer parametrically (e.g., well-known facts in training data). Over-retrieval adds latency, increases context length, and can introduce noise from retrieved passages that contradict the model's correct parametric knowledge.
- **Avoids under-retrieval:** Single-step RAG fails on multi-hop questions requiring information from multiple documents or reasoning chains spanning several retrieval steps. Adaptive RAG routes these to multi-step retrieval rather than forcing them through a single retrieve-then-generate pass.
- **Benchmark results:** Evaluated on open-domain QA benchmarks (PopQA, TriviaQA, MuSiQue, 2WikiMultiHopQA, WebQ, StrategyQA). Adaptive RAG matches or outperforms both always-retrieve and always-multi-step baselines across datasets while using fewer retrieval calls on average.

## Connections

Adaptive RAG builds on [[rag|standard RAG]] by adding a routing layer. The multi-step retrieval branch it routes to is architecturally related to iterative RAG and [[self-rag|Self-RAG]] — in both cases, the model issues multiple retrieval calls, updating its reasoning with each retrieved batch. The key difference is that Self-RAG decides whether to retrieve at each generation step via special tokens; Adaptive RAG decides up front via a classifier.

[[corrective-rag|Corrective RAG]] is complementary: Corrective RAG focuses on assessing and correcting the quality of retrieved passages, while Adaptive RAG focuses on whether to retrieve at all and how many steps to take. The two could be combined — Adaptive RAG routing followed by Corrective RAG quality assessment within each branch.

For VaultMind, Adaptive RAG suggests a routing layer upstream of the retrieval pipeline. Many agent queries will be operational (e.g., "run a command", "format this output") and require no vault retrieval. Others will be factual lookups answerable from a single note. A few will require assembling context from multiple related notes. Implementing adaptive routing would allow VaultMind to skip retrieval entirely for queries the agent can handle parametrically, reducing latency and avoiding the risk of irrelevant vault content being injected into context. The complexity classifier could be approximated by the agent itself using a lightweight prompt classification step before invoking VaultMind search, or VaultMind could expose a routing API that the agent uses to decide which retrieval strategy to invoke.
