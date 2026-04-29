---
id: concept-action-potential
type: concept
title: Action Potential
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Nerve impulse
  - Spike
  - AP
tags:
  - neuroscience
  - electrophysiology
related_ids:
  - concept-neuron
  - concept-ion-channels
  - concept-membrane-potential
  - concept-synaptic-transmission
  - concept-hebbian-learning
source_ids:
  - source-hodgkin-huxley-1952
  - source-wikipedia-action-potential
  - source-kandel-2012
---

## Overview

An action potential is the fast, all-or-none, self-regenerating voltage pulse a [[neuron|Neuron]] produces when its membrane is depolarized past threshold. It travels along the axon at roughly constant amplitude (typically peaking near +30 to +40 mV from a resting baseline of about -70 mV) and is the primary long-distance signal in the nervous system.

The mechanism was worked out quantitatively by Hodgkin and Huxley (1952) on the squid giant axon. Voltage-clamp experiments revealed that the spike is produced by a stereotyped, voltage-dependent dance between two main conductances — a fast-activating, fast-inactivating sodium current and a slower-activating potassium current — embodied in a small system of differential equations that still anchors computational neuroscience.

## Key Mechanisms

- **Threshold (~ -55 mV):** Depolarizing input must summate enough to open a critical mass of voltage-gated Na+ channels; below threshold, the membrane simply relaxes back. See [[ion-channels|Ion Channels]].
- **Depolarization (rising phase):** Once threshold is crossed, Na+ channels open regeneratively — Na+ rushes in, depolarizing further, opening more channels — driving the membrane toward the Na+ equilibrium potential.
- **Repolarization (falling phase):** Na+ channels inactivate while voltage-gated K+ channels open; K+ efflux returns the membrane toward the K+ equilibrium potential.
- **Hyperpolarization / afterpotential:** K+ channels close slowly, briefly driving the membrane below the resting [[membrane-potential|Membrane Potential]].
- **All-or-none:** Above threshold the spike's shape is largely independent of stimulus strength. Information is encoded in spike timing and rate, not in spike size.
- **Refractory period:** During the absolute refractory period (Na+ channels inactivated) no new spike can fire; during the relative refractory period a stronger-than-normal stimulus is required. This enforces unidirectional propagation along the axon.
- **Propagation velocity:** Roughly 1 m/s in unmyelinated axons; 10-100+ m/s in myelinated axons via saltatory conduction at nodes of Ranvier.

## Connections

The action potential is the bridge between [[membrane-potential|Membrane Potential]] (the resting state and the Nernst/GHK machinery that sets it) and [[synaptic-transmission|Synaptic Transmission]] (the spike's arrival at the axon terminal triggers Ca2+-dependent neurotransmitter release). The Hodgkin-Huxley framework also makes the action potential the canonical example of how [[ion-channels|Ion Channels]] with voltage-dependent gating can implement a non-linear computation in a single cell.

For [[hebbian-learning|Hebbian Learning]], the action potential is what "fire" means in "fire together, wire together" — coincident pre- and post-synaptic spiking is the trigger condition for spike-timing-dependent plasticity.
