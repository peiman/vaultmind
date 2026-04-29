---
id: concept-loihi-neuromorphic-chip
type: concept
title: Intel Loihi Neuromorphic Chip
created: 2026-04-29
vm_updated: 2026-04-29
tags:
  - neuromorphic
  - hardware
  - bio-inspired-ai
related_ids:
  - concept-neuromorphic-computing
  - concept-spiking-neural-network
  - concept-leaky-integrate-and-fire
  - concept-spike-timing-dependent-plasticity
  - concept-hebbian-learning
source_ids:
  - source-davies-2018
  - source-wikipedia-neuromorphic
---

## Overview

Loihi is Intel's research neuromorphic processor, introduced by [[source-davies-2018|Davies et al. (2018)]]. Among the modern wave of [[neuromorphic-computing|neuromorphic chips]] it is distinguished primarily by *on-chip programmable learning*: unlike IBM TrueNorth, which is inference-only, Loihi can update synaptic weights in real time using configurable plasticity rules including pairwise [[spike-timing-dependent-plasticity|STDP]], triplet STDP, and reward-modulated STDP. That combination — programmable spiking neuron models, programmable plasticity, mesh-routed asynchronous spike traffic, and 14 nm-scale integration — made Loihi the most popular research substrate for closed-loop spiking-network experiments in the late 2010s and 2020s.

## How It Works

**Loihi 1 (2018):**

- 14 nm Intel process
- 128 neuromorphic cores per chip
- 130,000 spiking neurons total per chip
- 130 million synapses
- Asynchronous on-chip mesh; chips can be tiled into multi-chip systems
- Per-core local SRAM holding both neuron state and synaptic weights
- Programmable [[leaky-integrate-and-fire|LIF]]-style neuron with extensions
- Programmable learning rules built from pre/post spike traces

**Loihi 2 (2021):**

- 7 nm Intel 4 process
- ~1 million neurons per chip
- Fully programmable neuron model (microcode-level), not just LIF
- Faster spike messaging and improved scaling fabric
- Used as the building block for Hala Point (2024), 1152 chips, 1.15 B neurons

**Hala Point (2024):**

- 1.15 billion neurons across 1152 Loihi 2 chips
- Pitched as "human-cortex-scale" research substrate
- Argonne and Sandia among early users

## Why It Matters

Loihi makes [[spike-timing-dependent-plasticity|STDP]]-class biologically plausible learning a first-class on-chip operation, not a software simulation. That collapses the gap between neuroscience-inspired learning rules and deployable hardware. Reported workloads include LASSO sparse coding, graph search, constraint satisfaction, robotic arm control, and odor / gesture recognition — all running at single-digit milliwatts or tens of milliwatts, often 100-1000× less energy than GPU equivalents on the same task. The argument for [[neuromorphic-computing|neuromorphic computing]] is essentially "look at Loihi's numbers."

## Connections

Loihi is the canonical recent chip discussed in the parent [[neuromorphic-computing|neuromorphic computing]] note. The neurons it runs are [[leaky-integrate-and-fire|LIF]]-style (Loihi 1) or programmable (Loihi 2). The learning rule it implements is the modern variant of [[hebbian-learning|Hebbian learning]] — namely [[spike-timing-dependent-plasticity|STDP]] — and the network class it accelerates is the [[spiking-neural-network|SNN]].
