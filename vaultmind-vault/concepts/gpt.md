---
id: concept-gpt
type: concept
title: GPT (Generative Pre-trained Transformer)
created: 2026-04-29
aliases:
  - GPT
  - Generative Pre-trained Transformer
  - Decoder-Only Transformer
tags:
  - llms
  - deep-learning
  - pretraining
  - transformers
  - decoder-only
related_ids:
  - concept-bert
  - concept-transformer
  - concept-attention-mechanism
  - concept-scaling-laws
  - concept-instruction-tuning
  - concept-rlhf
source_ids:
  - source-radford-2018
---

## Overview

GPT (Generative Pre-trained Transformer) is the decoder-only autoregressive branch of the transformer family, originated by [[source-radford-2018|Radford et al. 2018]] at OpenAI. The recipe is simple: train a stack of decoder transformer blocks on raw text with a left-to-right language modeling objective (predict the next token given everything to its left), then deploy the same model directly for generation. Unlike [[bert|BERT]]'s pretrain-then-fine-tune-per-task recipe, GPT-lineage models are designed to be queried in their pretrained generative form — early via task-specific prompting, later via instruction tuning and chat.

The GPT lineage runs through GPT-1 (2018, 117M), GPT-2 (2019, 1.5B), GPT-3 (2020, 175B), InstructGPT (2022), ChatGPT (2022), GPT-4 (2023), GPT-4o, o1, o3, GPT-5. The same architectural skeleton scales across more than five orders of magnitude in parameter count, and that scalability — combined with the discovery of in-context learning and emergent capabilities — is the central empirical fact of modern AI.

## How It Works

**Architecture.** A stack of transformer decoder blocks. Each block has causal (lower-triangular) self-[[attention-mechanism|attention]] — token t can attend to tokens 1..t but not future tokens — followed by a feedforward (or [[mixture-of-experts|MoE]]) block. No encoder, no cross-attention. Position information via learned absolute positions (GPT-1/2), then RoPE, then ALiBi or YaRN in long-context variants.

**Pretraining.** Maximize log p(x_t | x_1..x_{t-1}) — the standard left-to-right language modeling loss. Train on huge corpora: web text, books, code, scientific papers. Modern frontier pretraining uses 10–15+ trillion tokens.

**In-context learning.** GPT-3 surfaced an unexpected capability: the model could perform new tasks by being shown a few examples in its prompt, with no weight updates. This few-shot learning ability is the core thing chat-style interaction with LLMs depends on.

**Post-training.** A modern GPT-lineage frontier model is the result of:
1. **Pretraining** — next-token prediction on the web-scale corpus.
2. **[[instruction-tuning|Instruction tuning (SFT)]]** — supervised fine-tuning on demonstrations.
3. **[[rlhf|RLHF]]** (or [[dpo|DPO]] / [[constitutional-ai|RLAIF]]) — preference optimization for helpfulness and harmlessness.
4. **(For reasoning models)** RL on chain-of-thought generation against process or outcome rewards.

**Inference.** Sample tokens one at a time, conditioned on the full prompt + previously sampled tokens. KV-caching makes this O(N) per token instead of O(N²) by reusing past key/value tensors.

## Recent Developments

The GPT lineage and its open siblings (LLaMA, Mistral, Qwen, DeepSeek, Gemma) drive almost all current LLM progress:

- **GPT-3 (2020)** — 175B; established in-context learning and the emergent-capabilities phenomenology.
- **InstructGPT / ChatGPT (2022)** — RLHF brought GPT to mass usability.
- **GPT-4 (2023)** — multimodal, dramatically better reasoning; rumored MoE architecture (~1.8T total / ~280B active).
- **LLaMA family (2023+)** — Meta's open-weights line; the dominant base for community fine-tunes and research.
- **o1 / o3 (2024–25)** — reasoning models with internalized long chains of thought trained via RL on process rewards.
- **GPT-4o, Gemini 2, Claude 3.5–4.7** — natively multimodal frontier models.
- **DeepSeek-V3 / R1 (2024–25)** — open-weights frontier capability with novel MoE designs and reasoning training.

## Connections

GPT and [[bert|BERT]] are the two great branches of the [[transformer|transformer]] family. GPT's left-to-right generative regime makes it the natural fit for chat, code, agents, and reasoning. BERT-lineage encoders dominate dense retrieval and classification.

Almost every other concept in this batch composes with GPT: [[scaling-laws|scaling laws]] govern its training; [[mixture-of-experts|MoE]] expands its capacity; [[flash-attention|FlashAttention]] makes attention tractable at frontier scale; [[instruction-tuning|instruction tuning]] and [[rlhf|RLHF]] / [[dpo|DPO]] / [[constitutional-ai|CAI]] align it; [[chain-of-thought|CoT]] and [[tree-of-thought|ToT]] extract its reasoning; [[mamba-state-space-models|Mamba]] is the most credible architectural challenger.

For VaultMind, every generative call (the `vaultmind ask` answer composition, the optional reranker rationale) goes through a GPT-lineage model. The retrieval stack is BERT-lineage; the generation stack is GPT-lineage; they meet at the context window.
