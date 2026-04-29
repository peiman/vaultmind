---
id: concept-multielectrode-array-recording
type: concept
title: Multi-Electrode Array Recording
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - MEA
  - Utah array
  - Large-scale electrophysiology
tags:
  - neuroscience
  - methods
  - electrophysiology
  - bmi
related_ids:
  - concept-neuron
  - concept-action-potential
  - concept-neuropixels
  - concept-two-photon-imaging
  - concept-pyramidal-neurons
source_ids:
  - source-jun-2017
---

## Overview

Multi-electrode array (MEA) recording is the simultaneous extracellular measurement of [[action-potential|Action Potentials]] from many neurons using a fixed grid or shank of electrodes. From single tungsten wires in the 1950s, the technology has progressed through tetrodes (4-wire bundles, ~20 isolated cells), Utah arrays (~100 silicon needles, the workhorse of human BMI), and now CMOS-integrated probes with hundreds to thousands of densely spaced sites. The result is a "neural data revolution": where one paper used to record dozens of neurons, a single 2024 paper routinely reports thousands.

## How It Works

- **Extracellular signal:** Each electrode picks up the local field potential plus the spikes of nearby (~50–100 µm) neurons, which appear as ~1 ms biphasic waveforms at ~30 kHz sampling.
- **Spike sorting:** Software (KiloSort, Mountainsort, Spyking Circus) clusters spike waveforms across electrodes to assign each spike to a putative single neuron. Dense arrays improve sorting because the same spike appears on multiple sites with a characteristic pattern.
- **Form factors:**
  - *Utah array:* 10×10 silicon needles, each one electrode at the tip — used in primate and human cortex for decades.
  - *Tetrode bundles:* Independent 4-wire moving bundles, gold standard for chronic hippocampal recording.
  - *Silicon multi-shank probes:* (e.g., NeuroNexus, Cambridge NeuroTech) Multiple shanks each with linear electrode arrays.
  - *Neuropixels:* CMOS-integrated single shank with hundreds of densely tiled sites — see [[neuropixels|Neuropixels]].

## Key Capabilities

- **Population dynamics:** Simultaneous recording exposes correlated activity, sequence replay, and population-level computations invisible to single-unit work.
- **Closed-loop and BMI:** Decoded population spikes drive prosthetic limbs and speech BMIs in tetraplegic humans (BrainGate program with Utah arrays).
- **Layer- or depth-resolved recording:** Linear probes span cortical layers, distinguishing superficial from deep-layer [[pyramidal-neurons|Pyramidal Neurons]].
- **Chronic stability:** Many arrays remain useful for months to years, enabling longitudinal studies of learning and aging.

## Recent Findings

- **Steinmetz et al. (Science 2019):** Eight Neuropixels probes simultaneously recorded ~30,000 neurons across mouse brain regions in a single behavioral task — the first truly brain-wide single-unit dataset.
- **International Brain Laboratory (IBL) Brain-Wide Map (2023–2024):** Standardized Neuropixels recordings from 139 brain regions across 547 mice performing the same task, a community resource analogous to the Allen atlas for electrophysiology.
- **Human single-unit BMIs:** Utah-array decoders now produce intelligible speech in real time from motor cortex spikes (Willett et al., 2023; Metzger et al., 2023).
- **Drug-screening and organoid MEAs:** High-density CMOS MEAs (3DBrainMEA, MaxOne) record from cultured neurons and organoids, supporting in vitro pharmacology and disease modeling.

## Connections

[[neuropixels|Neuropixels]] is the current state-of-the-art instance of the multi-electrode array idea — same goal (many neurons, one shank), advanced realization. MEA recording is the temporal counterpart to [[two-photon-imaging|Two-Photon Imaging]]: trades cellular identity and chronic re-identification for millisecond resolution and depth.

For VaultMind, MEA data is the empirical foundation for any claim about precise spike-timing, replay, or sequence learning that the vault references in its [[memory-replay|Memory Replay]] and [[hebbian-learning|Hebbian Learning]] notes — those phenomena are visible only with sub-millisecond population recording.
