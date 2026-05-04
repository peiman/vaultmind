---
id: concept-leaky-integrate-and-fire
type: concept
title: Leaky Integrate-and-Fire (LIF) Neuron
created: 2026-04-29
tags:
  - bio-inspired-ai
  - spiking-neural-network
  - computational-neuroscience
related_ids:
  - concept-spiking-neural-network
  - concept-izhikevich-model
  - concept-neuron
  - concept-action-potential
  - concept-membrane-potential
  - concept-ion-channels
  - concept-neuromorphic-computing
source_ids:
  - source-wikipedia-lif
  - source-maass-1997
---

## Overview

The leaky integrate-and-fire (LIF) neuron is the simplest computational model that captures the essential behavior of a spiking [[neuron|neuron]]: integrate incoming current onto a [[membrane-potential|membrane potential]], leak it away over time, fire a spike when threshold is crossed, and reset. It throws away the molecular detail of [[ion-channels|ion channels]] and the shape of the [[action-potential|action potential]] itself, keeping only the input-output relationship that matters for downstream computation. Because it is fast, analytically tractable, and good enough for many questions, LIF is the default neuron in large [[spiking-neural-network|SNN]] simulations and in most digital [[neuromorphic-computing|neuromorphic chips]].

## Key Mechanism

The LIF neuron is an RC circuit: a capacitance (membrane) charged by input current, drained by a leak resistance to a resting potential.

```
C_m · dV/dt = -(V − V_rest) / R_m + I(t)
if V ≥ V_threshold:  emit spike, set V = V_reset, hold reset for refractory τ_ref
```

Three parameters dominate behavior:

- **Membrane time constant τ_m = R_m · C_m** — how fast the neuron forgets past input. Short τ_m → coincidence detector (only near-simultaneous inputs add up). Long τ_m → integrator (sums input over a longer window).
- **Threshold V_threshold** — the firing decision boundary.
- **Refractory period τ_ref** — caps maximum firing rate at 1/τ_ref.

The firing rate as a function of constant input current I is

```
f(I) = 1 / (τ_ref + τ_m · ln(R_m · I / (R_m · I − V_threshold)))
```

A clean, rate-saturating f-I curve. The simplest extension — adaptive LIF (AdEx, generalized LIF) — adds a slow adaptation variable to reproduce spike-frequency adaptation, which plain LIF lacks.

## How It Works in Practice

Vector form for a population of N LIF neurons with weights W:

1. Compute input currents I = W · s_pre(t), where s_pre is the pre-synaptic spike vector.
2. Update membranes: V ← V + dt · (-(V − V_rest)/τ_m + I/C_m).
3. Spike test: s = (V ≥ V_threshold). For neurons that spiked, V ← V_reset, start refractory countdown.
4. Repeat.

This loop is the core of frameworks like Brian, NEST, and snnTorch, and the neuron primitive baked into [[loihi-neuromorphic-chip|Loihi]], TrueNorth, and SpiNNaker.

## Trade-offs vs. Other Models

- vs. Hodgkin-Huxley: LIF is ~100-1000× cheaper but cannot reproduce sub-threshold oscillations, bursting, or precise spike shapes.
- vs. [[izhikevich-model|Izhikevich]]: same order of cost, but Izhikevich captures ~20 firing patterns LIF cannot.
- vs. analog [[perceptron|perceptron]]: LIF is event-driven, has internal state, and uses time as a computational dimension.

## Connections

LIF is the workhorse for [[spiking-neural-network|SNN]] research. Its lack of a biological action-potential shape doesn't matter when downstream circuits care only about spike timing — exactly the regime [[spike-timing-dependent-plasticity|STDP]] operates in. In [[neuromorphic-computing|neuromorphic hardware]] the LIF update is the thing being implemented in custom silicon, often as a fixed-point integer increment plus a comparator. For VaultMind-style activation models, LIF is the cleanest worked example of "leak + integrate + threshold" — the same pattern used in [[base-level-activation|ACT-R base-level activation]] decay and in [[spreading-activation|spreading activation]] thresholding, just with very different time constants.
