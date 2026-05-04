---
id: concept-neuropixels
type: concept
title: Neuropixels Probes
created: 2026-04-29
aliases:
  - Neuropixels
  - IMEC silicon probe
tags:
  - neuroscience
  - methods
  - electrophysiology
  - cmos
related_ids:
  - concept-multielectrode-array-recording
  - concept-neuron
  - concept-action-potential
  - concept-two-photon-imaging
  - concept-optogenetics
source_ids:
  - source-jun-2017
---

## Overview

Neuropixels is a family of CMOS-integrated silicon probes — developed at imec in collaboration with HHMI Janelia, Allen Institute, UCL and the Wellcome Trust — that record extracellular spikes from hundreds of neurons across the depth of the brain on a single ~10 mm-long, 70×20 µm shank. The first-generation probe (Jun et al., Nature 2017) carries 960 recording sites with 384 simultaneously selectable channels; each channel digitizes at ~30 kHz on-probe, eliminating the analog cabling that used to limit channel count.

Neuropixels collapsed two long-standing constraints of large-scale electrophysiology — channel count and probe size — at the same time. A single mouse implant now yields data quantities comparable to a 2010-era Utah-array primate study. See [[multielectrode-array-recording|Multi-Electrode Array Recording]] for the broader family.

## How It Works

- **CMOS shank:** All amplification, multiplexing and ADCs are on-chip; the probe outputs a digital data stream rather than ~1000 analog wires.
- **Site density:** Sites are 12 µm × 12 µm at 20 µm pitch in a checkerboard, dense enough that each spike appears on multiple sites — the spatial signature that modern spike sorters (KiloSort) exploit.
- **Selectable channels:** Software chooses which 384 of the 960 sites to record, allowing layer- or region-targeted configurations within a single insertion.
- **Form factors:** NP 1.0 (single shank, mouse-friendly), NP 2.0 (4 shanks × 1280 sites, smaller, chronic-implantable), NP-Ultra (8 µm pitch, sub-cellular resolution), and human-grade variants for intraoperative use.

## Key Capabilities

- **Hundreds of neurons per probe, brain-wide with multiple probes:** A single session with 2–8 probes routinely yields several thousand well-isolated single units across many regions.
- **Chronic re-implantation:** NP 2.0 supports stable months-long recordings in freely behaving mice, enabling longitudinal study of learning and plasticity.
- **Layer- and column-resolved analyses:** Linear geometry distinguishes superficial vs. deep [[pyramidal-neurons|Pyramidal Neurons]] in the same column.
- **Compatibility with optical methods:** Probes are routinely combined with [[optogenetics|Optogenetics]] for opto-tagging of cell types and with [[two-photon-imaging|Two-Photon Imaging]] for cross-modal validation.

## Recent Findings

- **Steinmetz et al. (Science 2019), Brain-wide neural activity:** 8 Neuropixels in mice performing a visual task recorded ~30,000 neurons — the first brain-wide single-unit dataset.
- **International Brain Laboratory (IBL) (2023–2024):** Standardized Neuropixels recordings from 139 mouse brain regions across 547 animals on the same task — an open community dataset that has become a benchmark for systems neuroscience.
- **Neuropixels 2.0 (Steinmetz et al., Science 2021):** 4-shank, smaller form factor, chronic across months in mice and rats.
- **Neuropixels-Ultra (2024):** 8 µm site pitch resolves sub-somatic features and improves cell-type classification from spike waveform.
- **Human acute recordings:** Paulk et al. (2022), Chung et al. (2022) used Neuropixels in awake human cortex during epilepsy and tumor surgeries, recording hundreds of single units per insertion.

## Connections

Neuropixels is the current best instance of [[multielectrode-array-recording|Multi-Electrode Array Recording]] for *in vivo* mammalian work. It is the temporal-resolution complement to [[two-photon-imaging|Two-Photon Imaging]] (millisecond spikes vs. ~10 Hz calcium) and the readout side of all-electrical [[optogenetics|Optogenetics]] experiments.

For VaultMind, Neuropixels is the canonical example of a method whose existence reshaped what counts as a *single experiment*: the unit of analysis went from one neuron to one population. The vault uses it as a reference point in notes about scaling — the same way one might invoke ImageNet when discussing dataset-driven inflection points in machine learning.
