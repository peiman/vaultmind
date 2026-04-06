---
id: source-asai-2023
type: source
title: "Asai, A., Wu, Z., Wang, Y., Sil, A., & Hajishirzi, H. (2023). Self-RAG: Learning to Retrieve, Generate, and Critique through Self-Reflection. ICLR 2024."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2310.11511"
aliases:
  - Asai 2023
  - Self-RAG paper
tags:
  - retrieval
  - self-reflection
related_ids:
  - concept-self-rag
  - concept-rag
  - concept-reflexion
---

# Asai et al. — Self-RAG (ICLR 2024)

Asai et al. trained a single language model to interleave retrieval decisions, generation, and self-critique using special reflection tokens. The model is fine-tuned on data augmented with four token types: `[Retrieve]` (should retrieval happen here?), `[IsRel]` (is the retrieved passage relevant?), `[IsSup]` (does the generated text faithfully use the passage?), and `[IsUse]` (is this response useful to the user?). During inference, the model emits these tokens as part of its normal output stream, enabling retrieval calls to be triggered on-demand rather than for every generation step.

Training uses a critic model (GPT-4 in the original work) to annotate a large corpus of (query, output, passage) triples with reflection token labels, which are then distilled into the target model via standard supervised fine-tuning. No reinforcement learning is required. The result is a single model that can serve as both the generator and the retrieval gating function.

At ICLR 2024, Self-RAG outperformed ChatGPT (gpt-3.5-turbo) and retrieval-augmented Llama 2-13B on PopQA, ASQA, PubHealth, Arc-Challenge, and TriviaQA. It also outperformed perplexity-prompted baselines that always retrieve, confirming that selective retrieval improves over always-on retrieval.

## Key Findings

- Outperforms ChatGPT on open-domain QA, fact verification, long-form generation, and reasoning tasks using a 13B Llama 2 backbone
- Always-on retrieval is suboptimal: gating retrieval with `[Retrieve]` tokens improves both accuracy and efficiency
- Reflection token quality is sensitive to the critic model; GPT-4-annotated data outperforms weaker critics significantly
- Beam search over reflection token candidates at inference time further improves performance beyond greedy decoding
- Self-RAG generalizes across task types without task-specific training — a single model handles QA, reasoning, and long-form generation

## Relevance to VaultMind

Self-RAG directly addresses VaultMind's retrieval efficiency problem. Currently VaultMind retrieves whenever a query crosses a relevance threshold, but this heuristic is brittle. Self-RAG's training paradigm offers a path to a VaultMind agent that decides when to query the vault based on learned task signals rather than a fixed threshold. The `[IsRel]` and `[IsSup]` critique tokens also map naturally to VaultMind's need to assess whether retrieved note content actually supports a generated response — a mechanism for reducing confabulation from irrelevant vault content.
