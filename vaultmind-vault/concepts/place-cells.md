---
id: concept-place-cells
type: concept
title: Place Cells
created: 2026-04-26
tags:
  - neuroscience
  - hippocampus
  - spatial-cognition
  - episodic-memory
related_ids:
  - concept-grid-cells
  - concept-time-cells
  - concept-pyramidal-neurons
  - concept-memory-engrams
  - concept-neural-replay
  - concept-sharp-wave-ripples
  - concept-episodic-memory
source_ids:
  - source-okeefe-dostrovsky-1971
---

## Overview

A **place cell** is a hippocampal pyramidal neuron — typically in CA1 or CA3 — that fires preferentially when an animal is in a specific small region of its environment, called the cell's *place field*, and is nearly silent everywhere else. Different place cells have different place fields, and across a population the fields tile the entire environment, so that the animal's current location can be decoded from which subset of place cells is active.

Discovered by John O'Keefe and Jonathan Dostrovsky in 1971 ([[source-okeefe-dostrovsky-1971|paper]]) and later canonised in O'Keefe and Nadel's *The Hippocampus as a Cognitive Map* (1978), place cells are the foundational evidence that the hippocampus implements an allocentric (world-centred) spatial map rather than a sensory or motor map. The discovery contributed to O'Keefe's share of the 2014 Nobel Prize in Physiology or Medicine.

## How It Works

Each place cell fires in one (or occasionally a few) compact regions of an arena, regardless of the animal's heading or running speed. The firing depends on a combination of distal landmarks, proprioceptive / path-integration input, and learned context. When the animal enters a new environment, place cells "remap": each one chooses a new field (or falls silent), generating a fresh population code for the new context.

The code is sparse — at any instant a small fraction of CA1 cells are active — and population-distributed: location is read out from *which* cells fire, not from how fast any one fires. Place fields are stable across days when the environment is unchanged but reorganise instantly when it changes (rotation, rescaling, or new room).

## Key Findings

- **Allocentric coding:** Place fields are anchored to the room, not the animal's body axis.
- **Sparse population code:** ~30-50% of CA1 pyramidal cells are place cells in a given environment; any single location activates only a few.
- **Remapping:** Two distinct environments produce two essentially independent population codes ("global remapping"), giving the hippocampus a way to keep contexts separate.
- **Theta phase precession:** As an animal runs through a place field, the cell's spikes occur at progressively earlier phases of the 6-10 Hz theta rhythm, packing temporal sequence information into single theta cycles (O'Keefe & Recce 1993).
- **Replay during rest:** Place-cell sequences from waking trajectories are reactivated, in compressed form, during subsequent sleep — see [[neural-replay|Neural Replay]] and [[sharp-wave-ripples|Sharp-Wave Ripples]].

## Recent Developments

- **Beyond rodents:** Place cells have been recorded in epilepsy patients implanted with depth electrodes (Ekstrom et al. 2003), in bats (Yartsev & Ulanovsky 2013, including 3D fields and goal-direction cells), and in marmosets navigating in VR.
- **Non-spatial coding:** Aronov, Nevers & Tank (2017) showed that place cells in rats trained on a non-spatial sound-frequency task encode position along the *auditory* dimension. The "place" in place cells generalises to any continuous task variable; "place" was always a special case of episodic context.
- **Goal cells and reward fields:** A subset of CA1 cells preferentially fire near goal locations, biasing the map toward behaviourally important regions.
- **Concept cells:** Quian Quiroga's "Jennifer Aniston" cells in human medial temporal lobe (hippocampus, amygdala) fire to a specific person across pictures, names, and voices — interpretable as place cells with a much abstracter "place" axis.

## Connections

Place cells receive their spatial input largely from [[grid-cells|Grid Cells]] in the medial entorhinal cortex; they share circuitry with [[time-cells|Time Cells]] (often the same cells, retasked for temporal coding); they are the cellular substrate of episodic location memory and are recruited as the spatial component of [[memory-engrams|Memory Engrams]]. Their replay during [[sharp-wave-ripples|Sharp-Wave Ripples]] is the most-studied example of [[neural-replay|Neural Replay]].

For VaultMind, place cells are a model of content-addressable, sparse, remappable storage. Vector-space neighbourhoods are the engineered analogue: a query activates a sparse subset of "fields" in the embedding space, and the same store can host many independent contexts (vaults), each with its own remapping.
