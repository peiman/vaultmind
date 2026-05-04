---
id: concept-sharp-wave-ripples
type: concept
title: Sharp-Wave Ripples
created: 2026-04-26
tags:
  - neuroscience
  - hippocampus
  - oscillations
  - consolidation
  - sleep
related_ids:
  - concept-neural-replay
  - concept-place-cells
  - concept-memory-consolidation
  - concept-memory-replay
  - concept-pyramidal-neurons
  - concept-interneurons
source_ids:
  - source-buzsaki-2015
  - source-wilson-mcnaughton-1994
---

## Overview

A **sharp-wave ripple (SPW-R)** is a brief (50-150 ms), highly synchronous population event in the hippocampus, consisting of a slow "sharp wave" of depolarisation in CA3 paired with a fast (140-250 Hz) "ripple" oscillation in CA1. Tens of thousands of pyramidal cells discharge in tightly choreographed sequences during each event. SPW-Rs are the most synchronous spontaneous pattern in the mammalian brain, and they are now widely regarded as the offline computational engine of memory consolidation and planning.

Discovered and named by György Buzsáki in the 1980s, SPW-Rs spent two decades as an electrophysiological curiosity before [[source-wilson-mcnaughton-1994|Wilson & McNaughton (1994)]] showed that hippocampal place-cell sequences from waking trajectories are replayed inside them during sleep. Buzsáki's 2015 review ([[source-buzsaki-2015|paper]]) consolidates the modern view of SPW-Rs as a "cognitive biomarker" for episodic memory and planning.

## How It Works

SPW-Rs originate in CA3, whose dense recurrent excitatory connectivity supports the auto-associative dynamics that produce a sharp wave. The wave depolarises CA1, where a network of fast-spiking parvalbumin-positive [[interneurons|interneurons]] generates the ripple-frequency oscillation that organises pyramidal-cell spike timing. Within a single SPW-R, CA1 pyramidal cells fire in compressed temporal sequences — typically replaying ~1 second of waking experience in 100-150 ms, a 10× compression that is well-matched to spike-timing-dependent plasticity windows.

SPW-Rs occur during quiet wakefulness (consummatory pauses, immobility) and non-REM sleep. Their output spreads beyond hippocampus to neocortex, basal ganglia, ventral striatum, and prefrontal cortex, providing a system-wide synchronisation event during which the rest of the brain can integrate hippocampal output.

## Key Findings

- **Replay payload:** SPW-Rs carry temporally compressed replay of waking place-cell sequences, both forward and reverse ([[source-wilson-mcnaughton-1994|Wilson & McNaughton 1994]]; Foster & Wilson 2006; Diba & Buzsáki 2007).
- **Causal role in consolidation:** Selective electrical disruption of SPW-Rs during sleep impairs spatial memory in rats (Girardeau et al. 2009; Ego-Stengel & Wilson 2010).
- **Causal role in planning:** Disrupting awake SPW-Rs impairs upcoming choice behaviour (Jadhav et al. 2012).
- **Cortical coupling:** SPW-Rs are nested within neocortical slow oscillations and sleep spindles during NREM, the temporal scaffold for hippocampal-cortical dialogue (Sirota et al., Mölle et al.).
- **Long-duration ripples:** Fernández-Ruiz et al. (2019) showed that artificially prolonged SPW-Rs *enhance* memory, complementing the disruption studies.

## Recent Developments

- **Brain-wide ripples:** Ngo, Khorasani et al. (2020) and others have mapped SPW-R-locked activity across the entire brain in rodents; SPW-Rs co-occur with cortical "high-frequency oscillations" in many cortical regions.
- **Selection of experience for memory:** Yang et al. (2024, Science) showed that hippocampal sharp-wave ripples occurring shortly after experience tag which experiences will be remembered the next day — SPW-Rs are not just consolidation, but selection.
- **Closed-loop disruption / enhancement:** Real-time ripple-detection systems can either disrupt ripples (impairing memory) or trigger external stimulation phase-locked to them (enhancing memory).
- **Ripple-like events in humans:** Intracranial recordings in epilepsy patients show analogous fast oscillations in human hippocampus that increase around successful recall (Norman et al. 2019, *Science*).
- **Pathology — "p-ripples":** Distorted ripples are a marker of epileptogenic tissue and appear in mouse models of schizophrenia and Alzheimer's disease ([[source-buzsaki-2015|Buzsáki 2015]]).

## Connections

SPW-Rs are the carrier wave for [[neural-replay|Neural Replay]] and the leading mechanism for hippocampal contribution to [[memory-consolidation|Memory Consolidation]]. They reorganise [[place-cells|place-cell]] sequences for offline transmission to neocortex, are paced by an inhibitory network of [[interneurons|interneurons]], and are tightly linked to [[memory-engrams|Memory Engrams]] (engram cells preferentially participate in post-learning ripples).

For VaultMind, SPW-Rs are the canonical biological argument for *event-driven, compressed-time batch consolidation*: not a constant trickle, but discrete bursts during idle periods that move information from labile to stable storage. The current periodic re-index plus spaced review job is a coarse engineering analogue.
