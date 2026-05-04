---
id: concept-single-cell-rna-seq-brain
type: concept
title: Single-Cell RNA-Seq of the Brain
created: 2026-04-29
aliases:
  - scRNA-seq brain
  - Single-nucleus RNA-seq
  - Brain cell taxonomy
tags:
  - neuroscience
  - methods
  - transcriptomics
  - cell-types
related_ids:
  - concept-neuron
  - concept-pyramidal-neurons
  - concept-interneurons
  - concept-dopaminergic-neurons
  - concept-connectomics
source_ids:
  - source-tasic-2018
---

## Overview

Single-cell RNA sequencing (scRNA-seq) applied to brain tissue measures the transcriptome of thousands to millions of individual cells, then clusters them into transcriptomic *types* by similarity in gene expression. In the last decade this has replaced morphology and electrophysiology as the dominant axis on which neuronal cell types are defined. Where earlier classifications named perhaps a dozen cortical interneuron classes, transcriptomic taxonomies now resolve hundreds of types per cortical area, organized into stable hierarchies that recur across species.

For brain tissue specifically, single-nucleus RNA-seq (snRNA-seq) is often used because adult [[neuron|Neurons]] are too large and fragile to dissociate cleanly; nuclei survive freezing and homogenization, expanding the method to archival post-mortem human brains.

## How It Works

- **Dissociation or nucleus prep:** Fresh brain tissue is enzymatically dissociated to single cells, or frozen tissue is homogenized to release nuclei.
- **Encapsulation:** Each cell/nucleus is captured in a droplet (10x Genomics Chromium) or microwell with a uniquely barcoded bead. mRNA is reverse-transcribed with the bead's barcode incorporated.
- **Pooled sequencing:** All cells' cDNA is pooled, sequenced together, then demultiplexed by barcode to recover a per-cell expression vector.
- **Clustering:** Dimensionality reduction (PCA → UMAP) followed by graph-based clustering (Leiden, Louvain) groups cells into transcriptomic types. Marker genes are extracted per cluster.
- **Cross-modal anchoring:** Patch-seq (recording, then aspirating, then sequencing the same cell) and MERFISH/spatial transcriptomics align transcriptomic clusters with electrophysiology, morphology, and anatomical position.

## Key Capabilities

- **Unsupervised cell-type discovery:** Cells cluster by their full expression profile rather than a small marker panel — types can be found that no marker gene would have suggested.
- **Cross-area and cross-species comparison:** The same pipeline applied to many regions/species yields directly comparable taxonomies.
- **Atlas scale:** The Allen Brain Atlas and BICCN (BRAIN Initiative Cell Census Network) have profiled tens of millions of cells across the mouse and human brain, producing reference atlases used as ground truth in downstream studies.

## Recent Findings

- **Tasic et al. (Nature 2018):** Sequenced 23,822 cells from mouse primary visual cortex and anterior lateral motor cortex, defining 133 transcriptomic types. GABAergic [[interneurons|Interneurons]] are largely shared across both areas while glutamatergic [[pyramidal-neurons|Pyramidal Neurons]] are mostly area-specific — and projection target can be predicted from transcriptomic type.
- **BICCN whole-mouse-brain atlas (Yao et al., 2023):** ~7 million cells across the entire mouse brain, ~5,200 clusters, ~300 supertypes — the most complete neuronal taxonomy to date.
- **Human cortex atlases (Hodge et al. 2019; Jorstad et al. 2023):** Show that human cortical cell types are conserved with mouse at the class/subclass level but diverge sharply at the finest cluster level, especially in supragranular layers.
- **Patch-seq integration:** Direct evidence that transcriptomic types map onto distinct firing patterns and morphologies, justifying the taxonomy as a unified cell-type definition.

## Connections

Transcriptomic taxonomy is now the standard input to other modern methods: [[connectomics|Connectomics]] uses cell-type labels to interpret the wiring diagram, [[optogenetics|Optogenetics]] and [[chemogenetics-dreadds|Chemogenetics]] depend on cell-type-specific Cre lines whose specificity is validated by scRNA-seq, and [[two-photon-imaging|Two-Photon Imaging]] increasingly reads out activity from transcriptomically defined populations.

For VaultMind, the lesson of brain scRNA-seq is structural: cell-type *clusters* emerge from unsupervised similarity over high-dimensional features, exactly the operation VaultMind performs over note embeddings. The cell taxonomies are an existence proof that meaningful, hierarchical types can be recovered from raw vectors without hand-curated labels — a useful precedent for argument the vault makes in its retrieval and ranking notes.
