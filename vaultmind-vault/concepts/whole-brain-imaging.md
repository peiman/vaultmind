---
id: concept-whole-brain-imaging
type: concept
title: Whole-Brain Imaging
created: 2026-04-29
aliases:
  - Light-sheet microscopy
  - Tissue clearing
  - CLARITY
  - iDISCO
  - Expansion microscopy
tags:
  - neuroscience
  - methods
  - imaging
  - tissue-clearing
related_ids:
  - concept-neuron
  - concept-two-photon-imaging
  - concept-connectomics
  - concept-pyramidal-neurons
source_ids:
  - source-chung-2013
  - source-ahrens-2013
---

## Overview

Whole-brain imaging is a family of techniques for visualizing every cell in an entire brain (or an entire small animal) at single-cell resolution. The shared problem is that brain tissue scatters light, so deeper than ~1 mm conventional fluorescence microscopy fails. The shared solution is to *make the brain transparent*, *image it from the side instead of through it*, or *physically enlarge it* — and ideally to combine these.

The three pillars are:

- **Tissue clearing** — chemical methods that match the refractive index of brain tissue to a transparent medium, removing scattering. Examples: CLARITY (Chung & Deisseroth 2013), iDISCO+ (Renier et al. 2014), CUBIC (Susaki et al. 2014).
- **Light-sheet fluorescence microscopy (LSFM)** — illuminates a single thin plane from the side and images it from above, eliminating out-of-focus exposure. Enables fast, low-photodamage volumetric imaging of cleared or naturally transparent samples.
- **Expansion microscopy (ExM)** — physically enlarges the sample 4–20× by embedding it in a swellable polymer, turning a diffraction-limited problem into a geometric one (Chen, Tillberg & Boyden, 2015).

## How It Works

- **CLARITY:** Tissue is infused with a hydrogel monomer, polymerized into a mesh that locks proteins and nucleic acids in place, then lipids are removed electrophoretically. The result is a transparent, macromolecule-permeable brain compatible with antibody staining.
- **iDISCO+ / 3DISCO:** Solvent-based dehydration plus refractive-index matching with dibenzyl ether. Faster and simpler than CLARITY but tissue shrinks.
- **CUBIC:** Aqueous reagents using aminoalcohols to remove lipids and quench heme — preserves fluorescent protein signal better than solvent methods.
- **Light-sheet microscope (e.g., Ahrens 2013, Janelia SiMView):** A cylindrical lens or scanned Gaussian beam forms a thin sheet; the orthogonal detection objective captures the whole illuminated plane in one camera frame. A whole larval zebrafish brain at single-cell resolution images in ~1 second.
- **Expansion microscopy:** A sample is crosslinked to a swellable polyacrylate gel, proteinase-digested to anchor labels to the gel, then water-swelled to enlarge isotropically. A standard confocal then resolves features below the conventional diffraction limit.

## Key Capabilities

- **Cellular-resolution whole-brain anatomy:** Every [[neuron|Neuron]] expressing a fluorescent label can be located in 3D, allowing brain-wide cell counts and projection mapping.
- **Brain-wide functional imaging in transparent animals:** Light-sheet plus GCaMP records every neuron in the larval zebrafish brain at ~1 Hz volumetric rates (Ahrens et al., Nat Methods 2013).
- **Multi-round molecular interrogation:** CLARITY-cleared brains can be stained, washed, and re-stained, allowing many proteins or transcripts to be probed in the same intact volume.
- **Sub-diffraction resolution on standard microscopes:** Expansion microscopy gives effective ~25 nm resolution on a confocal — a major democratization of super-resolution.

## Recent Findings

- **Brain-wide projection atlases:** AAV tracing combined with iDISCO+ light-sheet imaging produced cell-type-resolved projection maps of mouse cortex (Allen Mouse Brain Connectivity Atlas, Oh et al. 2014; Harris et al. 2019).
- **Whole-mouse-body imaging (vDISCO, Cai et al. 2019):** Cleared and imaged entire adult mice, allowing whole-body neural and tumor-cell mapping at cellular resolution.
- **Expansion + light-sheet (ExLSFM):** Gao et al. (Science 2019) combined 4× expansion with lattice light-sheet to image *Drosophila* brains and cultured cells at nanoscale across millimeters of volume.
- **Iterative expansion (X10, X20):** Pushes effective resolution toward ~10 nm — close to volume EM but with molecular labeling.

## Connections

Whole-brain imaging supplies the *anatomical* and *brain-wide functional* substrate that point methods like [[two-photon-imaging|Two-Photon Imaging]] and [[neuropixels|Neuropixels]] sample from. For *connectivity* at synapse resolution, [[connectomics|Connectomics]] (volume EM) is still the gold standard; light-sheet on cleared tissue trades synapse resolution for whole-brain coverage and molecular labeling.

For VaultMind, the most relevant lesson is structural: every advance here is a *coverage* advance — the same imaging principles applied to bigger volumes faster — and the resulting datasets share a graph-of-cells-with-attributes shape that is directly analogous to the vault's notes-with-properties graph.
