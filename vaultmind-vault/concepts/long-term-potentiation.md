---
id: concept-long-term-potentiation
type: concept
title: Long-Term Potentiation (LTP)
created: 2026-04-29
tags:
  - neuroscience
  - plasticity
  - learning
related_ids:
  - concept-hebbian-learning
  - concept-long-term-depression
  - concept-spike-timing-dependent-plasticity
  - concept-synaptic-transmission
  - concept-action-potential
  - concept-pyramidal-neurons
  - concept-ion-channels
  - concept-memory-consolidation
source_ids:
  - source-bliss-lomo-1973
  - source-wikipedia-ltp
---

## Overview

Long-term potentiation (LTP) is a persistent strengthening of synaptic transmission produced by patterns of activity that drive the post-synaptic neuron strongly while the pre-synaptic neuron is firing. It is the cellular substrate most directly implicated in learning and memory storage in the mammalian brain, and the experimental confirmation — decades later — of Hebb's 1949 postulate. The discovery is classically attributed to [[bliss-lomo-1973|Bliss and Lomo (1973)]] in rabbit hippocampus, building on Lømo's 1966 observation in Per Andersen's lab.

## Key Mechanism

LTP at the canonical hippocampal Schaffer-collateral / CA1 [[pyramidal-neurons|pyramidal]] synapse follows the same pattern in many cortical circuits:

1. **Glutamate release.** Pre-synaptic [[action-potential|action potential]] triggers vesicle fusion; glutamate diffuses across the cleft (see [[synaptic-transmission|synaptic transmission]]).
2. **AMPA receptor activation.** Glutamate opens AMPA-type receptors on the post-synaptic spine, depolarizing it.
3. **NMDA receptor unblock.** NMDA receptors are coincidence detectors: they need both glutamate binding *and* post-synaptic depolarization to expel the Mg²⁺ block from the pore.
4. **Calcium influx.** Once unblocked, NMDA receptors admit a sharp Ca²⁺ transient into the spine — the trigger for LTP.
5. **Signaling cascade.** CaMKII auto-phosphorylates and persists as the molecular memory; PKA, PKC, and MAPK pathways activate; AMPA receptors are inserted into the post-synaptic membrane (early LTP); transcription and protein synthesis consolidate the change (late LTP, hours+).

The induction protocol matters: high-frequency tetanus (100 Hz × 1s) and theta-burst (bursts of 4 spikes at 100 Hz, repeated at 5 Hz) are the standard recipes; in vivo, naturally occurring patterns produce qualitatively similar effects.

## Properties

- **Input specificity** — only the synapses that were active during induction strengthen.
- **Associativity** — a weak input can be potentiated if it coincides with strong activation at the same neuron (the substrate for [[hebbian-learning|associative learning]]).
- **Cooperativity** — a single weak pathway often cannot induce LTP alone; multiple converging inputs can.
- **Persistence** — early LTP lasts ~1-3 hours; late LTP requires protein synthesis and lasts hours to days, with experimental reports out to weeks.

## Recent Developments

LTP is the most studied form of synaptic plasticity, but the field has moved well past the Schaffer-collateral textbook story. NMDAR-independent LTP forms exist (mossy fiber to CA3, cerebellar parallel fiber). Behavioral studies link LTP-like changes to specific learning episodes via tagging and capture protocols and via optogenetic erasure / restoration of memory traces (Tonegawa lab, 2014). Pathologically, soluble Aβ oligomers in Alzheimer's disease impair LTP in the hippocampus before plaque deposition, providing a synaptic-level account of early cognitive symptoms.

## Connections

LTP is one half of the bidirectional plasticity story; [[long-term-depression|LTD]] is the other. [[spike-timing-dependent-plasticity|STDP]] is the time-resolved generalization that integrates both into a single learning rule keyed to spike timing. At the systems level LTP feeds into [[memory-consolidation|memory consolidation]], where hippocampal traces are reactivated and gradually transferred to neocortex. The Bliss & Lomo paper sits at the foundation of computational accounts of [[hebbian-learning|Hebbian learning]] used in cognitive architectures and modern bio-inspired AI.
