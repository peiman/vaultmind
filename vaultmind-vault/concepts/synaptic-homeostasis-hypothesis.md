---
id: concept-synaptic-homeostasis-hypothesis
type: concept
title: Synaptic Homeostasis Hypothesis
created: 2026-04-29
aliases:
  - SHY
  - Synaptic Downscaling During Sleep
  - Tononi-Cirelli Hypothesis
tags:
  - neuroscience
  - sleep
  - synaptic-plasticity
  - memory-consolidation
  - homeostasis
related_ids:
  - concept-slow-wave-activity
  - concept-sleep-stages
  - concept-memory-consolidation
  - concept-hebbian-learning
  - concept-long-term-potentiation
  - concept-spike-timing-dependent-plasticity
  - concept-active-systems-consolidation
source_ids:
  - source-tononi-cirelli-2006
  - source-tononi-cirelli-2014
  - source-rasch-born-2013
---

## Overview

The **Synaptic Homeostasis Hypothesis (SHY)**, proposed by Giulio Tononi and Chiara Cirelli, holds that the primary function of sleep is to renormalise synaptic strength following the net synaptic potentiation that accumulates during waking learning. In their account, wakefulness — driven by noradrenaline, acetylcholine, and sensory input — promotes broad Hebbian strengthening of synapses throughout the brain. Because this cannot continue indefinitely without saturating the system and consuming unsustainable energy, sleep serves as a global "downscaling" phase: synapses are scaled down toward a lower baseline, erasing weak or noise-driven potentiation while preserving proportional differences (and thus relative memories).

The hypothesis makes two interlinked predictions. First, **slow-wave activity (SWA) indexes synaptic load**: SWA should increase with prior waking (reflecting accumulated potentiation) and decrease across sleep (reflecting downscaling). Second, **sleep should be locally regulated**: brain regions that were more active during waking should show more SWA recovery. Both predictions have substantial empirical support.

## How It Works

SHY rests on the idea that **slow oscillations are the cellular read-out of synaptic strength**: stronger average synaptic connections produce larger UP-state depolarisations (and thus larger EEG slow waves), while weaker connections after downscaling produce smaller waves. Downscaling itself is proposed to occur via homeostatic synaptic scaling mechanisms (e.g., AMPA receptor removal, spine retraction) promoted by the synaptic silence of DOWN states.

The molecular route proposed involves several candidates: reduced AMPA receptor surface expression during sleep, downregulation of GluA1 phosphorylation, and cortisol-mediated suppression of LTP-maintenance cascades. More recent work has focused on the Homer1a protein, which translocates to synapses during sleep and suppresses mGluR5 signalling, mechanistically driving downscaling.

**Energy rationale:** Tononi and Cirelli calculated that the metabolic cost of maintaining the waking level of synaptic potentiation overnight would be prohibitive. Sleep allows a periodic reset that restores the dynamic range of the system — preparing it for another round of learning.

## Key Findings

- **SWA homeostasis:** SWA increases with sleep deprivation and decreases across the sleep night in proportion to its initial level — the clearest human evidence for homeostatic regulation ([[source-tononi-cirelli-2006|Tononi & Cirelli 2006]]).
- **Local SWA:** Regions that practiced a specific skill (e.g., arm rotation) show locally elevated SWA over the corresponding motor cortex during subsequent sleep (Huber et al. 2004) — and this local SWA boost predicts next-day performance gains.
- **Molecular evidence:** Homer1a knockout mice show disrupted SWA and impaired synaptic downscaling; cortical synaptic strength (measured by miniature EPSCs) increases after sleep deprivation and decreases after sleep in rodents (de Vivo et al. 2017).
- **Spine morphology:** De Vivo et al. (2017, *Science*) directly measured 6,920 synapses by electron microscopy in mouse cortex and found that synapse size and axon-spine interface area decreased ~18% from post-wake to post-sleep — direct structural evidence for downscaling.

## Recent Developments

- **Tension with active consolidation:** SHY predicts net synaptic weakening during sleep; the [[active-systems-consolidation|Active Systems Consolidation]] hypothesis predicts selective strengthening at specific synapses (for consolidated memories). These are not mutually exclusive — SHY can be the background trend while potentiation occurs at selected trace synapses — but the empirical debate is active (Rasch & Born 2013).
- **SHY extension:** The "Synaptic Homeostasis with Memory" elaboration (Frank 2012; Tononi & Cirelli 2019) proposes that important memories are "tagged" during encoding and protected from downscaling, while untagged synapses are depressed. This preserves information while restoring dynamic range.
- **REM sleep role in SHY:** Tononi and Cirelli initially focused on NREM; subsequent work suggests REM may serve a distinct refinement function — pruning synapses that were downscaled insufficiently during NREM.

## Connections

SHY explains why [[slow-wave-activity|slow-wave activity]] is tightly coupled to the amount of prior learning and explains the homeostatic regulation of [[sleep-stages|sleep stages]]. It partially conflicts with and partially complements the [[active-systems-consolidation|Active Systems Consolidation]] hypothesis: both theories need [[hebbian-learning|Hebbian mechanisms]] and both invoke NREM sleep, but they differ on whether the net synaptic change during sleep is depression or potentiation.

For VaultMind, SHY maps cleanly to the idea of **scheduled pruning**: not all embeddings, links, and graph edges should be preserved forever. Periodic downscaling — merging near-duplicate nodes, removing low-confidence links, pruning stale content — restores the resolution of the knowledge graph and prepares it for the next round of learning, just as sleep restores the dynamic range of cortical circuits.
