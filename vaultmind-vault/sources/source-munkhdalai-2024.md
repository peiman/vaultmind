---
id: source-munkhdalai-2024
type: source
title: "Munkhdalai, T., Faruqui, M., & Gopal, S. (2024). Leave No Context Behind: Efficient Infinite Context Transformers with Infini-attention. arXiv:2404.07143."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2404.07143"
aliases:
  - Munkhdalai 2024
  - Infini-attention paper
tags:
  - long-context
  - attention
related_ids:
  - concept-infini-attention
  - concept-memorizing-transformers
---

# Munkhdalai, Faruqui, & Gopal — Infini-attention (2024)

Munkhdalai, Faruqui, and Gopal (Google) propose Infini-attention, a modification to the standard Transformer attention layer that incorporates a fixed-size compressive long-term memory alongside standard causal dot-product attention. The architecture processes sequences in segments: local attention operates over the current segment (standard causal attention), while a linear associative memory accumulates compressed representations of all prior segments. A learned scalar gate mixes the outputs of the two paths. The full sequence history is thus accessible through the compressive memory at O(1) additional memory cost — total memory does not grow with sequence length.

The paper evaluates 1B and 8B parameter models on BookSum (long-document summarization), where models must summarize entire books. Both scales achieve state-of-the-art results among models of comparable parameter count. A separate experiment demonstrates successful passkey retrieval at 1 million tokens — the model reliably recovers a numeric passkey embedded deep in a 1M-token input, confirming that information is not lost despite the compressive memory.

## Key Findings

- 1B and 8B Infini-attention models achieve SOTA on BookSum long-document summarization, outperforming prior long-context approaches at the same parameter counts
- Successful passkey retrieval at 1 million token sequence length — compressive memory retains sufficient information for precise recall under ideal conditions
- The learned gate β varies by layer: some layers rely primarily on local attention, others on long-term memory, suggesting spontaneous specialization
- Infini-attention is a drop-in replacement for standard attention; existing pre-trained weights can be continued-trained with the infini-attention modification at modest compute cost
- Compressive memory uses an associative memory update rule (outer-product write, linear read) — equivalent to a linear attention kernel applied to all past tokens

## Relevance to VaultMind

Infini-attention directly addresses the architectural constraint that motivates VaultMind's existence: limited context window length. As models adopt compressive long-term memory, agents can retain coherent summaries of very long interaction histories without external retrieval. For [[context-pack|Context Pack]], this suggests a shift in design: rather than retrieving full note content, a future system might retrieve compressed representations — letting the model's internal memory fill in details. The paper also reinforces that not all past context is equally recoverable from compressive memory, which preserves the case for [[rag|RAG]]-style explicit retrieval when high-fidelity recall of specific details is required.
