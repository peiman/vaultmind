---
id: source-liu-2023-ring
type: source
title: "Liu, H., Zaharia, M., & Abbeel, P. (2023). Ring Attention with Blockwise Transformers for Near-Infinite Context. arXiv:2310.01889."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2310.01889"
aliases:
  - Liu 2023 Ring Attention
  - Ring Attention paper
tags:
  - long-context
  - distributed-systems
related_ids:
  - concept-ring-attention
  - concept-infini-attention
---

# Liu, Zaharia, & Abbeel — Ring Attention (2023)

Liu, Zaharia, and Abbeel (UC Berkeley) introduce Ring Attention, a distributed algorithm for computing exact Transformer self-attention over sequences of arbitrary length across multiple accelerator devices. The input sequence is partitioned into blocks, one block per device arranged logically in a ring. Each device computes blockwise attention over its local block, while simultaneously sending its key-value pairs to the next device in the ring and receiving KV pairs from the previous device. This overlapping of communication with computation hides the inter-device latency, keeping the distributed algorithm efficient.

Because all KV pairs from all devices are eventually processed by every device (traversing the full ring), the resulting attention is mathematically identical to single-device full attention — no approximation is introduced. The paper demonstrates scaling to sequences 64× longer than a single device can handle alone, using a ring of 64 Cloud TPU v4 devices.

## Key Findings

- Sequences up to 64× single-device capacity are demonstrated using 64 TPUs, with linear scaling in achievable sequence length as device count increases
- No approximation: the algorithm computes exact softmax attention, bit-for-bit equivalent to single-device full attention
- Communication-compute overlap is key to efficiency: KV block transmission is pipelined with local blockwise attention computation, keeping utilization high
- Blockwise attention (the local primitive) follows Flash Attention-style memory-efficient computation, keeping per-device HBM usage bounded
- Causal masking is handled correctly by the distributed formulation — no special treatment required beyond correct assignment of block positions
- The approach generalizes beyond self-attention: cross-attention variants for encoder-decoder models are also supported

## Relevance to VaultMind

Ring Attention is a hardware scaling solution rather than an algorithmic approximation, which has implications for how VaultMind's value proposition evolves. As exact long-context inference becomes routinely available (through Ring Attention or similar distributed approaches), the raw capacity constraint on context windows weakens. The remaining challenge — "what should the agent attend to, given that everything could fit?" — is precisely the curation and relevance-ranking problem that [[context-pack|Context Pack]] addresses. Ring Attention does not eliminate the need for intelligent note selection; it raises the stakes for getting curation right, since poorly ranked content now wastes larger and more expensive context windows.
