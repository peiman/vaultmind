---
id: concept-spike-timing-dependent-plasticity
type: concept
title: Spike-Timing-Dependent Plasticity (STDP)
created: 2026-04-29
vm_updated: 2026-04-29
tags:
  - neuroscience
  - plasticity
  - bio-inspired-ai
  - learning
related_ids:
  - concept-hebbian-learning
  - concept-long-term-potentiation
  - concept-long-term-depression
  - concept-spiking-neural-network
  - concept-leaky-integrate-and-fire
  - concept-izhikevich-model
  - concept-neuromorphic-computing
  - concept-loihi-neuromorphic-chip
  - concept-action-potential
  - concept-synaptic-transmission
  - concept-backpropagation
source_ids:
  - source-bi-poo-1998
  - source-wikipedia-stdp
---

## Overview

Spike-timing-dependent plasticity (STDP) is a biological learning rule in which the change in synaptic strength depends on the precise relative timing of pre- and post-synaptic spikes. It is the modern, time-resolved form of [[hebbian-learning|Hebbian learning]]: not just "neurons that fire together wire together," but "the one that fired first reinforces the connection to the one that fired second." The rule was characterized definitively by [[source-bi-poo-1998|Bi and Poo (1998)]] in cultured rat hippocampal neurons and has since been observed across many cortical and subcortical regions.

## Key Mechanism

Pair an [[action-potential|action potential]] in the presynaptic neuron with one in the postsynaptic neuron, separated by a time offset Δt = t_post − t_pre. Repeat dozens of times. The induced weight change Δw follows an antisymmetric exponential window:

```
Δw = +A_+ · exp(−Δt / τ_+)   if Δt > 0     (pre-leads-post → potentiation)
Δw = −A_− · exp(+Δt / τ_−)   if Δt < 0     (post-leads-pre → depression)
```

with τ_+, τ_− on the order of 10-20 ms. Pre-before-post potentiates the synapse — consistent with the pre-synaptic input contributing to causing the post-synaptic spike. Post-before-pre depresses it — the input arrived too late to matter and is, on this evidence, decorrelated from the output.

Mechanistically, STDP rides on top of [[long-term-potentiation|LTP]] and [[long-term-depression|LTD]]: the decisive variable is post-synaptic Ca²⁺ concentration through NMDA receptors, which depends on coincidence between [[synaptic-transmission|glutamate release]] and post-synaptic depolarization. Sharp Ca²⁺ rise → LTP; modest Ca²⁺ rise → LTD; near-zero → no change.

## Variants

- **Triplet STDP** — pairwise rules cannot reproduce all rate-dependence in cortex; triplet rules (pre-pre-post and pre-post-post) do.
- **Reward-modulated STDP (R-STDP)** — gates the weight update by a global neuromodulatory signal (dopamine, ACh), turning STDP into a credit-assignment rule for reinforcement learning.
- **Inhibitory STDP** — different shapes at GABAergic synapses; often symmetric.
- **Voltage- or calcium-based STDP** — replaces spike timing with continuous voltage or calcium traces; more biophysically grounded.

## Why It Matters

STDP is the leading candidate for a *biologically plausible* learning rule, in contrast to [[backpropagation|backpropagation]], which requires non-local error signals that no biological mechanism is known to deliver. STDP is local (uses only pre- and post-synaptic activity at the synapse), unsupervised (no teacher), and online (one update per pairing). These properties make it the canonical learning rule on [[neuromorphic-computing|neuromorphic hardware]] — Intel's [[loihi-neuromorphic-chip|Loihi]] implements pairwise and reward-modulated STDP in silicon — and the default plasticity rule in [[spiking-neural-network|SNN]] research.

## Connections

- Substrate: [[long-term-potentiation|LTP]] and [[long-term-depression|LTD]] are the molecular implementations of the two halves of the STDP window.
- Lineage: classical [[hebbian-learning|Hebbian learning]] generalized to time-resolved spike pairs.
- Carriers: [[spiking-neural-network|SNNs]] using [[leaky-integrate-and-fire|LIF]] or [[izhikevich-model|Izhikevich]] neurons.
- Hardware: [[loihi-neuromorphic-chip|Loihi]] makes STDP a first-class on-chip operation.
- Contrast: gradient-based [[backpropagation|backprop]] is the second-generation learning rule STDP is most often compared against.
