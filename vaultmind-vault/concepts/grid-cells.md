---
id: concept-grid-cells
type: concept
title: Grid Cells
created: 2026-04-26
tags:
  - neuroscience
  - entorhinal-cortex
  - spatial-cognition
  - population-coding
related_ids:
  - concept-place-cells
  - concept-time-cells
  - concept-pyramidal-neurons
  - concept-neural-manifolds
source_ids:
  - source-hafting-2005
---

## Overview

A **grid cell** is a neuron in the medial entorhinal cortex (mEC) — primarily in layer II — that fires whenever the animal occupies *any* vertex of a regular triangular (hexagonal) lattice tiling the entire environment. Unlike a [[place-cells|place cell]], which has a single place field, a single grid cell fires at many locations, and those locations are arranged in a precise, periodic pattern that wraps the whole arena.

Grid cells were discovered by Torkel Hafting, Marianne Fyhn, Sturla Molden, May-Britt Moser and Edvard Moser in 2005 ([[source-hafting-2005|paper]]), one of the most consequential single neural-recording results of the twenty-first century. The Mosers shared the 2014 Nobel Prize in Physiology or Medicine with John O'Keefe.

## How It Works

Each grid cell is characterised by three parameters: the spacing between vertices, the orientation of the lattice, and the phase (an offset of the lattice from a reference point). Cells in the same anatomical "module" of mEC share spacing and orientation but differ in phase, collectively tiling space at one scale. Modules at progressively more ventral mEC have progressively larger spacings — roughly doubling, in a discrete set of scales.

A small population of grid cells with two or more different scales can uniquely specify any point in a plane, like a multi-frequency basis decomposition. Grid cells project to hippocampal CA1 / CA3, where they help drive [[place-cells|place cell]] firing.

## Key Findings

- **Hexagonal periodicity:** Firing fields form a near-perfect equilateral triangular lattice — strikingly regular for a biological signal ([[source-hafting-2005|Hafting et al. 2005]]).
- **Modular structure:** Grid cells cluster into discrete modules with shared spacing and orientation; spacings scale geometrically along the dorsoventral axis.
- **Path-integration anchored:** Grid maps persist briefly in darkness and arise from self-motion (path integration), then anchor to landmarks when available.
- **Stability across environments:** A given grid cell typically maintains its scale and orientation across rooms; only the phase shifts.
- **Conjunctive cells:** Some mEC cells fire with grid pattern *and* head-direction tuning, providing a vector-style code for movement.

## Recent Developments

- **Grids in humans:** Doeller, Barry & Burgess (2010) showed fMRI signatures of grid-like coding in human entorhinal cortex during virtual navigation. Jacobs et al. (2013) recorded grid cells directly in epilepsy patients.
- **Grids for non-spatial dimensions:** Constantinescu, O'Reilly & Behrens (2016) found grid-like fMRI signal as humans navigated an *abstract* 2D feature space (bird-shape morph), suggesting grid coding generalises to conceptual spaces.
- **Tolman-Eichenbaum Machine (Whittington et al. 2020):** A mathematical / neural-network framework that derives grid-cell-like representations from the requirement to generalise structure across novel environments.
- **Distorted grids:** In trapezoidal or asymmetric environments, grid lattices warp and shear, revealing that the "grid" is a learned solution, not a hard-coded crystal.
- **Banino et al. 2018 (Nature):** Deep RL agents trained to navigate developed grid-like representations spontaneously in their hidden layers, giving a normative argument that grids are a near-optimal basis for path integration.

## Connections

Grid cells project to [[place-cells|Place Cells]] and are widely thought to provide their metric scaffold. They live alongside head-direction, border, speed, and object-vector cells in the entorhinal-hippocampal navigation system. Their multi-scale, periodic basis bears a family resemblance to wavelets, Fourier features, and sinusoidal positional encodings in transformers — and to [[neural-manifolds|Neural Manifolds]] more generally as low-dimensional bases for high-dimensional behaviour.

For VaultMind, grid cells suggest that a small number of multi-scale basis functions can index a large content space efficiently. Rotary / sinusoidal position encodings, multi-resolution graph embeddings, and HNSW's layered structure are all close cousins of this idea.
