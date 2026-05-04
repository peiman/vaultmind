---
id: concept-neuromorphic-computing
type: concept
title: Neuromorphic Computing
created: 2026-04-29
tags:
  - neuromorphic
  - hardware
  - bio-inspired-ai
related_ids:
  - concept-loihi-neuromorphic-chip
  - concept-spiking-neural-network
  - concept-leaky-integrate-and-fire
  - concept-izhikevich-model
  - concept-spike-timing-dependent-plasticity
  - concept-perceptron
  - concept-multilayer-perceptron
  - concept-backpropagation
source_ids:
  - source-merolla-2014
  - source-davies-2018
  - source-furber-2014
  - source-wikipedia-neuromorphic
---

## Overview

Neuromorphic computing is the design of hardware whose primitive operations mirror the brain's: spiking neurons, plastic synapses, asynchronous event-driven communication, and co-located memory and compute. The framing was introduced by Carver Mead in the late 1980s, originally with analog VLSI circuits whose transistor physics were used to emulate neuronal dynamics directly. Today the term covers a much broader design space — analog, digital, mixed-signal, and emerging-device (memristor, phase-change) — united by the goal of running [[spiking-neural-network|SNN]] workloads at orders-of-magnitude better energy than CPUs/GPUs.

The pitch is structural: conventional von Neumann machines pay a heavy energy cost to shuttle data between memory and arithmetic units, and they spend cycles on every neuron every step. Neuromorphic chips co-locate weights with the neurons that use them and spend energy only when a spike actually fires. For sparse, event-driven workloads — sensor processing, robotic control, certain learning tasks — this can mean 100-1000× less power for comparable accuracy.

## Key Hardware Generations

- **Carver Mead's analog VLSI** (1980s-90s) — silicon retina, silicon cochlea, sub-threshold analog circuits used directly to model dendritic integration.
- **IBM TrueNorth** (2014) — 5.4 B transistors, 1 M neurons, 256 M synapses, ~70 mW. Inference-only, rate-coded, fully digital and asynchronous. Established the scale-and-power baseline ([[source-merolla-2014|Merolla et al. 2014]]).
- **Manchester SpiNNaker** (2014) — many small ARM cores plus a custom packet-routing fabric optimized for spike traffic. Goal: simulate ~1% of human cortex in real time. General-purpose neuromorphic substrate ([[source-furber-2014|Furber et al. 2014]]).
- **Stanford Neurogrid** — analog mixed-signal subthreshold neurons, sub-watt for million-neuron networks.
- **Intel [[loihi-neuromorphic-chip|Loihi]]** (2018) — digital spiking, on-chip programmable [[spike-timing-dependent-plasticity|STDP]] and reward-modulated learning, 130k neurons per chip; Loihi 2 (2021) brings programmable neuron models and improved scalability ([[source-davies-2018|Davies et al. 2018]]).
- **BrainScaleS-2** (Heidelberg) — analog neurons running 1000× faster than biological time, with on-chip plasticity.
- **Hala Point** (2024) — Intel's Loihi 2-based system at 1.15 B neurons.
- **Memristor / phase-change synapses** — research-stage analog crossbars using emerging devices to implement plastic synapses in-situ.

## Design Tradeoffs

- **Analog vs digital.** Analog gets you closer to brain energy but is harder to manufacture at scale, sensitive to process variation, and lossier to program. Digital is reproducible and CMOS-friendly but loses some efficiency.
- **On-chip learning vs inference-only.** TrueNorth is inference-only; Loihi has programmable plasticity. On-chip learning enables closed-loop / online adaptation but increases area and complexity.
- **Sparse, event-driven vs dense.** Real efficiency wins assume sparse activity; dense workloads do not benefit much over a GPU.
- **Programming model.** SNN frameworks (Lava, snnTorch, NEST, Brian) and ANN-to-SNN conversion are the main approaches; programming neuromorphic hardware is still a research frontier.

## Why It Matters

Neuromorphic computing is the engineering surface where biology stops being a metaphor and starts being a circuit specification. It is the strongest existence proof that [[spiking-neural-network|SNNs]] are not merely a research curiosity — when run on the right substrate, they win on energy. For VaultMind-style cognitive architectures the relevance is conceptual: brains pay for activity, not for state; designs that follow that constraint scale differently than dense ANN inference.

## Connections

[[loihi-neuromorphic-chip|Loihi]] is the canonical recent chip and gets a dedicated note. The neurons running on these chips are typically [[leaky-integrate-and-fire|LIF]] or [[izhikevich-model|Izhikevich]]; the learning rule is typically [[spike-timing-dependent-plasticity|STDP]] or a variant. The relevant contrast class is the [[perceptron|perceptron]] / [[multilayer-perceptron|MLP]] / [[backpropagation|backprop]] lineage that runs on GPUs.
