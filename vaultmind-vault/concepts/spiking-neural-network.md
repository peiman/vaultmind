---
id: concept-spiking-neural-network
type: concept
title: Spiking Neural Network (SNN)
created: 2026-04-29
tags:
  - bio-inspired-ai
  - spiking-neural-network
  - neuroscience
  - computational-neuroscience
related_ids:
  - concept-leaky-integrate-and-fire
  - concept-izhikevich-model
  - concept-spike-timing-dependent-plasticity
  - concept-neuromorphic-computing
  - concept-loihi-neuromorphic-chip
  - concept-neuron
  - concept-action-potential
  - concept-membrane-potential
  - concept-perceptron
  - concept-multilayer-perceptron
  - concept-backpropagation
  - concept-hebbian-learning
source_ids:
  - source-maass-1997
  - source-izhikevich-2003
  - source-wikipedia-snn
---

## Overview

A spiking neural network (SNN) is a class of artificial neural network in which neurons communicate via discrete events — spikes — rather than continuous real-valued activations. Each neuron integrates incoming spikes onto a membrane state and emits its own spike when that state crosses a threshold; information is encoded in spike rates, in precise spike times, or in population patterns. Wolfgang Maass framed SNNs as the "third generation" of neural network models, after McCulloch-Pitts threshold gates and analog [[perceptron|perceptrons]]/[[multilayer-perceptron|MLPs]] — distinguished by the fact that spike timing is a computational variable, not just an implementation detail.

SNNs are simultaneously a model of biological computation and an engineering substrate. Biologically, they capture the fact that real cortical [[neuron|neurons]] communicate with [[action-potential|action potentials]] over a [[membrane-potential|membrane potential]] driven by [[ion-channels|ion channels]]. Computationally, their event-driven dynamics make them a natural fit for [[neuromorphic-computing|neuromorphic hardware]], where energy is spent only when something happens.

## How It Works

A standard SNN comprises:
- **Neuron model.** Each unit has internal state (membrane potential, often adaptation/refractory variables). The simplest is the [[leaky-integrate-and-fire|leaky integrate-and-fire]] (LIF) model. The [[izhikevich-model|Izhikevich model]] adds a second variable to reproduce ~20 cortical firing patterns at ~LIF cost. Hodgkin-Huxley sits at the biophysical end, AdEx and GLM in the middle.
- **Synapses.** Pre-synaptic spikes induce post-synaptic currents weighted by the synaptic strength; weights can be plastic via [[spike-timing-dependent-plasticity|STDP]] or other [[hebbian-learning|Hebbian]] rules.
- **Coding.** Rate coding (spikes per second), temporal coding (latency, phase, rank order), and population coding (which subset spiked) all carry information. Choice of code shapes both biological realism and engineering accuracy.
- **Training.** SNNs are notoriously hard to train with [[backpropagation|gradient descent]] because the spike threshold is non-differentiable. Modern approaches: surrogate gradients (smooth pseudo-derivatives at threshold), ANN-to-SNN conversion (train an [[multilayer-perceptron|MLP]]/[[convolutional-neural-network|CNN]] then discretize activations to spikes), and direct biological rules like STDP and reward-modulated STDP.

## Recent Developments

The accuracy gap with conventional ANNs has narrowed and disappeared on several tasks. Surrogate-gradient training in PyTorch-based frameworks (snnTorch, Norse, SpikingJelly) made SNNs reproducible at scale. On hardware, Intel's [[loihi-neuromorphic-chip|Loihi]] and Loihi 2, IBM TrueNorth, Manchester SpiNNaker, BrainScaleS-2, and Hala Point demonstrate million- to billion-neuron capacities at orders-of-magnitude lower power than GPU inference for the same workload — the strongest argument for SNNs as more than a biological curiosity.

## Connections

- Per-neuron details: [[leaky-integrate-and-fire|LIF]] is the minimal computational unit; [[izhikevich-model|Izhikevich]] is the rich-but-cheap workhorse.
- Plasticity: [[spike-timing-dependent-plasticity|STDP]] is the standard biologically plausible learning rule; [[long-term-potentiation|LTP]] and [[long-term-depression|LTD]] are its molecular substrate.
- Hardware: [[neuromorphic-computing|Neuromorphic computing]] and the [[loihi-neuromorphic-chip|Loihi chip]] specifically are the engineering surface.
- Contrast with ANN tradition: [[perceptron|Perceptron]] → [[multilayer-perceptron|MLP]] → [[backpropagation|backprop]] is the second-generation lineage Maass wrote against.
- Biology: [[neuron|neuron]], [[action-potential|action potential]], [[synaptic-transmission|synaptic transmission]], [[membrane-potential|membrane potential]], [[ion-channels|ion channels]], [[pyramidal-neurons|pyramidal neurons]].
