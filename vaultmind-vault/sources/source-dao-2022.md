---
id: source-dao-2022
type: source
title: "Dao et al. FlashAttention: Fast and Memory-Efficient Exact Attention with IO-Awareness (2022)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2205.14135"
tags:
  - deep-learning
  - transformers
  - systems
  - attention
related_ids:
  - concept-flash-attention
---

# Dao et al. — FlashAttention (2022)

Tri Dao, Daniel Fu, Stefano Ermon, Atri Rudra, and Christopher Ré (Stanford) reformulated the attention kernel as an IO-aware algorithm that minimizes reads and writes between GPU HBM and on-chip SRAM. Instead of materializing the full N×N attention matrix in HBM (the classical bottleneck), FlashAttention tiles the computation, computes softmax incrementally in SRAM via the online-softmax trick, and never instantiates the full matrix. The result is exact attention — no approximation — that is dramatically faster and uses linear memory in sequence length.

The paper produced 15% wall-clock speedup on BERT-large, 3× on GPT-2, and unlocked 64K-context training. FlashAttention-2 (2023) and FlashAttention-3 (2024) further refined the kernel; the IO-aware design is now the default attention implementation across PyTorch, JAX, and most serving stacks, and a precondition for long-context modern LLMs.
