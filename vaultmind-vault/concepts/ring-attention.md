---
id: concept-ring-attention
type: concept
title: Ring Attention
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Blockwise Ring Attention
  - Distributed Attention
tags:
  - long-context
  - distributed-systems
  - architecture
related_ids:
  - concept-infini-attention
  - concept-working-memory
source_ids:
  - source-liu-2023-ring
---

## Overview

Ring Attention (Liu, Zaharia, & Abbeel, UC Berkeley, 2023) is a distributed attention algorithm that enables Transformer training and inference over sequences far longer than the memory capacity of any single accelerator device. The core idea is to partition the input sequence across a ring of devices and compute blockwise attention, overlapping the communication of key-value blocks between adjacent devices with the local attention computation. Because communication is hidden behind compute, the effective memory cost per device drops to O(sequence_length / device_count), making sequence length scalable with the number of devices.

Critically, Ring Attention introduces no approximations: it computes exact attention over the full sequence, just distributed across hardware. This distinguishes it from sparse attention methods (which skip token pairs) and linear attention methods (which approximate softmax). The paper demonstrated sequences up to 64× longer than memory-efficient single-device transformers by using 64 TPUs.

Published as arXiv:2310.01889 in October 2023.

## Key Properties

- **Exact attention, no approximations:** Unlike sparse or linear attention, Ring Attention computes the full O(n²) attention matrix — just across devices. Numerical equivalence with single-device attention is guaranteed.
- **Communication-compute overlap:** KV blocks are passed around the ring while the current device computes attention over its local KV block. This hides communication latency, making the overhead of distribution small relative to compute.
- **Linear scaling with device count:** A ring of D devices can process sequences D times longer than a single device with the same per-device memory footprint.
- **Blockwise computation:** Builds on blockwise attention (Flash Attention-style memory efficiency) as its local primitive, then extends it to the distributed ring setting.
- **Causal masking handled correctly:** The blockwise ring formulation correctly handles autoregressive (causal) masking by assigning appropriate blocks to each device in the ring ordering.
- **Hardware-agnostic:** The algorithm applies to any accelerator topology that supports ring or all-reduce style collective communication (TPUs, multi-GPU clusters).

## Connections

Ring Attention is a hardware scaling solution for long context, orthogonal to architectural solutions like [[infini-attention|Infini-Attention]]. Infini-attention reduces memory by compressing past tokens; Ring Attention distributes exact attention across more hardware. They represent two ends of the long-context spectrum: approximate-but-cheap vs. exact-but-distributed.

The [[working-memory|Working Memory]] analogy is illuminating here: Ring Attention expands the capacity of the "attentional spotlight" without changing what that spotlight does — it just provides more real estate. In contrast, Infini-attention changes the nature of the spotlight itself.

For VaultMind, the practical consequence of Ring Attention (and long-context progress generally) is a gradual shift in the problem the system solves. When a model can attend to an entire codebase or note vault in a single forward pass with exact attention, the challenge is no longer "what fits in context" but "what deserves to be in context." This reframes VaultMind's core value proposition: not filling a small window, but curating a large one. Relevance ranking, noise reduction, and semantic organization become the critical tasks — not retrieval per se.
