---
id: concept-two-photon-imaging
type: concept
title: Two-Photon Calcium Imaging
created: 2026-04-29
aliases:
  - 2P imaging
  - Two-photon microscopy
  - In vivo calcium imaging
tags:
  - neuroscience
  - methods
  - imaging
  - calcium-indicators
related_ids:
  - concept-neuron
  - concept-action-potential
  - concept-pyramidal-neurons
  - concept-connectomics
  - concept-optogenetics
  - concept-whole-brain-imaging
source_ids:
  - source-wikipedia-two-photon
---

## Overview

Two-photon calcium imaging is the workhorse method for recording the activity of large populations of neurons in living, behaving animals. A pulsed near-infrared laser (~900–1000 nm) is focused into the brain through a cranial window; only at the focal point is photon density high enough for two infrared photons to be absorbed simultaneously, exciting a fluorophore as if a single visible photon had been absorbed. Because excitation is confined to a sub-femtoliter volume, scattered light does not produce out-of-focus background, and imaging depth into scattering tissue extends to ~1 mm.

Combined with genetically encoded calcium indicators (most prominently the GCaMP family), this gives a per-neuron readout of intracellular calcium, which closely tracks the rate of firing of [[action-potential|Action Potentials]]. Tens of thousands of cells can be monitored in a single session in awake mice running on treadmills or virtual-reality rigs.

## How It Works

- **Two-photon excitation** localizes excitation to the focal volume, suppressing background and enabling deep imaging in scattering tissue.
- **GCaMP indicators** are GFP variants fused to calmodulin and the M13 peptide; calcium binding triggers a conformational change that turns up green fluorescence ~10–30×.
- **Population recording:** A scanning galvo or resonant mirror raster-scans the focal point through a plane, yielding a movie of fluorescence per pixel. Modern variants (mesoscopes, multiplane / multi-region scanning) extend coverage to multiple millimeters or simultaneous depths.
- **Inference:** Calcium traces are deconvolved (e.g., OASIS, CaImAn) into estimated spike rates per neuron per frame.

## Key Capabilities

- Cellular-resolution recording of hundreds to tens of thousands of neurons simultaneously.
- Repeated imaging of the *same* identified cells across days to weeks — chronic tracking of plasticity and learning.
- Targetable to genetically defined cell types (Cre lines) so that [[pyramidal-neurons|Pyramidal Neurons]] and specific [[interneurons|Interneurons]] can be measured separately.
- Combinable with [[optogenetics|Optogenetics]]: red-shifted opsins are activated by a second laser while green GCaMP is read out, enabling all-optical perturb-and-record.

## Recent Findings

- **GCaMP6/7/8 series (Janelia, 2013–2023):** Successive indicator generations have brought single-spike sensitivity within reach and reduced response time toward 10 ms.
- **Cortical mesoscopes:** Custom microscopes (e.g., Sofroniew 2016, Demas 2021) image >5 mm of cortex at cellular resolution, enabling multi-area recording in mice running behavioral tasks.
- **MICrONS co-registration (2025):** Two-photon recordings of ~75,000 mouse V1 neurons were aligned to subsequent EM reconstruction of the same volume, producing the first large-scale function-wired-to-structure dataset (see [[connectomics|Connectomics]]).
- **3-photon imaging:** Pushes depth past 1 mm into hippocampus through intact cortex, at the cost of slower frame rates.

## Connections

Two-photon calcium imaging is the dominant *recording* technique for cortical population activity in mice; [[neuropixels|Neuropixels]] is the dominant *electrophysiological* alternative, with complementary trade-offs (2P trades temporal resolution for cellular identity and chronic stability; Neuropixels trades cellular identity for millisecond resolution and depth).

It is the natural readout for closed-loop [[optogenetics|Optogenetics]] experiments and for verifying the functional response of cells later mapped by [[connectomics|Connectomics]]. For VaultMind, 2P imaging is the empirical substrate behind every claim about cortical population codes that the vault references.
