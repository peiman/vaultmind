---
id: concept-izhikevich-model
type: concept
title: Izhikevich Neuron Model
created: 2026-04-29
vm_updated: 2026-04-29
tags:
  - bio-inspired-ai
  - spiking-neural-network
  - computational-neuroscience
related_ids:
  - concept-leaky-integrate-and-fire
  - concept-spiking-neural-network
  - concept-neuron
  - concept-action-potential
  - concept-pyramidal-neurons
  - concept-neuromorphic-computing
source_ids:
  - source-izhikevich-2003
  - source-wikipedia-snn
---

## Overview

Eugene Izhikevich's 2003 neuron model is the most widely used compromise between biological realism and computational efficiency. Two coupled ODEs and four parameters reproduce roughly twenty distinct cortical firing patterns — regular spiking, fast spiking, intrinsically bursting, chattering, low-threshold spiking, resonator, rebound spike/burst, frequency adaptation, bistability — that the [[leaky-integrate-and-fire|LIF]] model cannot, while costing only marginally more per step. Tens of thousands of Izhikevich neurons can be simulated in real time on a single CPU core.

## Key Mechanism

```
dv/dt = 0.04 v² + 5 v + 140 − u + I
du/dt = a (b v − u)

if v ≥ 30 mV:
    v ← c
    u ← u + d
```

- `v` represents the [[membrane-potential|membrane potential]].
- `u` is a recovery variable representing the activation of K⁺ and inactivation of Na⁺ [[ion-channels|ion currents]] — the same biophysics that shapes the [[action-potential|action potential]] downstroke and refractoriness.
- `a` is the recovery time scale.
- `b` is recovery sensitivity to subthreshold v.
- `c` is the post-spike reset of v (depth of after-hyperpolarization).
- `d` is the post-spike jump of u (controls spike-frequency adaptation and bursting).

Different (a, b, c, d) tuples reproduce different cortical cell types: regular spiking ([[pyramidal-neurons|pyramidal]] excitatory) at (0.02, 0.2, −65, 8); fast spiking (parvalbumin [[interneurons|interneurons]]) at (0.1, 0.2, −65, 2); intrinsically bursting at (0.02, 0.2, −55, 4); chattering at (0.02, 0.2, −50, 2). Izhikevich's 2003 paper includes a now-iconic figure showing all twenty patterns side-by-side.

## Why It Matters

The model occupies a sweet spot:

- **Hodgkin-Huxley** is biophysically faithful but too expensive for million-neuron simulations.
- **LIF** is cheap but can only spike at one rhythm.
- **Izhikevich** is roughly LIF-priced and can be made to spike like (almost) any cortical neuron.

This made it the default choice for large-scale brain simulations (Izhikevich's own 2008 thalamocortical model with 100,000 neurons is the canonical example) and a recurring building block in [[neuromorphic-computing|neuromorphic]] research designs where richer dynamics matter without paying full HH cost.

## Connections

The Izhikevich model is the natural step up from [[leaky-integrate-and-fire|LIF]] when LIF's single-rhythm behavior is too impoverished. It sits inside [[spiking-neural-network|SNNs]] and inherits everything those bring: [[spike-timing-dependent-plasticity|STDP]]-based plasticity, event-driven simulation, and natural deployment on [[neuromorphic-computing|neuromorphic hardware]]. The recovery variable `u` is where the model talks to real neurobiology — the same K⁺/Na⁺ [[ion-channels|channel dynamics]] that Hodgkin and Huxley nailed down quantitatively.
