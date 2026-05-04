---
id: concept-long-term-depression
type: concept
title: Long-Term Depression (LTD)
created: 2026-04-29
tags:
  - neuroscience
  - plasticity
  - learning
related_ids:
  - concept-long-term-potentiation
  - concept-hebbian-learning
  - concept-spike-timing-dependent-plasticity
  - concept-synaptic-transmission
  - concept-pyramidal-neurons
  - concept-ion-channels
source_ids:
  - source-wikipedia-ltd
  - source-bliss-lomo-1973
---

## Overview

Long-term depression (LTD) is the activity-dependent weakening of synapses that complements [[long-term-potentiation|LTP]]. Without an opposing process, repeated learning would saturate every synapse and erase contrast; LTD is the homeostatic counterpart that keeps the synaptic dynamic range usable. It is also a learning rule in its own right — most famously in the cerebellum, where LTD at parallel-fiber-to-Purkinje-cell synapses is the cellular substrate for motor-skill calibration.

## Key Mechanism

LTD takes different forms in different circuits, with the unifying theme that *modest* post-synaptic Ca²⁺ — rather than the sharp transient that triggers LTP — drives weakening:

- **Hippocampal NMDAR-LTD.** Low-frequency stimulation (1 Hz × 15 min) produces a small, prolonged Ca²⁺ rise that selectively activates phosphatases (PP1, calcineurin) instead of CaMKII, dephosphorylating AMPA receptors and triggering their endocytic removal from the [[synaptic-transmission|post-synaptic membrane]].
- **mGluR-LTD.** Group I metabotropic glutamate receptors trigger local protein synthesis and AMPAR internalization; relevant in fragile-X syndrome (FMR1 loss → exaggerated mGluR-LTD).
- **Cerebellar LTD.** Conjunctive activation of parallel fibers and the climbing fiber on a Purkinje cell produces a Ca²⁺ + DAG signal that internalizes AMPARs at the parallel-fiber synapse — the Marr-Albus-Ito mechanism for supervised motor learning.
- **Endocannabinoid-dependent LTD.** Retrograde endocannabinoid signaling depresses pre-synaptic release at many cortical and striatal synapses.

## Why It Matters

- **Homeostasis.** With only LTP, synapses run away to ceiling; LTD prevents saturation and preserves the dynamic range needed for further learning.
- **Forgetting and selection.** LTD provides a substrate for "use it or lose it" — synapses that are not co-active with their post-synaptic partner are pruned in strength.
- **The depression half of [[spike-timing-dependent-plasticity|STDP]].** When post-synaptic spikes precede pre-synaptic ones (post-before-pre, on a 10-20 ms scale), the Ca²⁺ profile favors LTD; this is the negative half of the STDP window characterized by [[source-bi-poo-1998|Bi & Poo (1998)]].
- **Cerebellar motor learning.** The Marr-Albus-Ito theory of cerebellar learning is built on LTD at parallel-fiber-Purkinje synapses guided by climbing-fiber error signals from the inferior olive — one of the cleanest examples of a supervised learning rule implemented in real biology.

## Recent Developments

Optogenetic erasure of LTD in specific circuits has been used to test which behaviors depend on it (e.g., motor adaptation, fear memory extinction). LTD is implicated in Alzheimer's pathology: soluble Aβ oligomers facilitate hippocampal LTD by impairing glutamate reuptake, biasing the LTP/LTD balance toward weakening — a candidate synaptic mechanism for early memory loss.

## Connections

[[long-term-potentiation|LTP]] and LTD together implement the two-direction synaptic-weight axis of [[hebbian-learning|Hebbian learning]]; [[spike-timing-dependent-plasticity|STDP]] formalizes how that axis maps onto pre/post spike timing. In bio-inspired AI, LTD provides the depression rule that keeps weights bounded under STDP-style learning on [[loihi-neuromorphic-chip|neuromorphic hardware]].
