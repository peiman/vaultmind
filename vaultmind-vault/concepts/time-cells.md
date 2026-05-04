---
id: concept-time-cells
type: concept
title: Time Cells
created: 2026-04-26
tags:
  - neuroscience
  - hippocampus
  - episodic-memory
  - temporal-coding
related_ids:
  - concept-place-cells
  - concept-grid-cells
  - concept-pyramidal-neurons
  - concept-episodic-memory
  - concept-memory-engrams
source_ids:
  - source-macdonald-2011
---

## Overview

A **time cell** is a hippocampal neuron that fires preferentially at a specific moment within a temporally structured experience — a delay period, a sequence of events, or the gap between two cues — much as a [[place-cells|place cell]] fires at a specific location within a spatial environment. Across a population, time cells tile the duration of the experience: cell A fires at second 1, cell B at second 3, cell C at second 7, and together they reconstruct *when* an event happened.

The term and the canonical demonstration come from MacDonald, Lepage, Eden & Eichenbaum (2011) ([[source-macdonald-2011|paper]]), though hints of temporal coding in CA1 ensembles date back to Pastalkova et al. (2008) and earlier work on theta-sequence coding.

## How It Works

In the [[source-macdonald-2011|MacDonald 2011]] task, rats sniffed an odour, held still for 10 seconds in a small enclosure, then made a choice. During the delay — when location, sensory input, and behaviour were nearly constant — CA1 ensembles produced stereotyped sequential activity in which each cell had its own preferred moment in the gap. Decoding from the population reconstructed elapsed time within ~100 ms.

When the delay was lengthened or shortened, time cells *retimed*: the sequence rescaled or remapped to fill the new interval, exactly mirroring how place cells remap when the environment rescales. This parallel — same circuit, different coding axis — is the strongest argument that the hippocampus implements a general "what / where / when" episode code, not a dedicated map of physical space.

## Key Findings

- **Sequential tiling:** Time-cell ensembles tile elapsed time during delays, mazes, and event sequences ([[source-macdonald-2011|MacDonald et al. 2011]]).
- **Retiming:** Changing the delay duration causes the time-cell sequence to rescale or remap, paralleling place-cell remapping.
- **Same cells, different axes:** Many cells act as time cells in some task phases and place cells in others, depending on which dimension the task makes informative.
- **Theta phase precession in time:** Pastalkova et al. (2008) showed that time-cell sequences also exhibit theta phase precession, suggesting the same theta-mediated sequencing mechanism that organises spatial trajectories also organises temporal ones.
- **Cross-species:** Time-cell-like activity has been recorded in rat, mouse, and human medial temporal lobe.

## Recent Developments

- **Multi-timescale:** Bright et al. (2020) and others have found time-cell-like sequences operating at multiple timescales simultaneously, from seconds to minutes — potentially a logarithmic basis for episodic time.
- **Time + place + odor conjunctive coding:** Single CA1 cells can encode conjunctions of where, when, and what, supporting Eichenbaum's view that episodic memory is fundamentally about *relations* among these dimensions.
- **Lateral entorhinal contribution:** Tsao et al. (2018, Nature) found time-coding signals in lateral entorhinal cortex, the "what" stream into hippocampus — suggesting the temporal scaffolding starts upstream of CA1.
- **Computational models:** Howard & Eichenbaum's TILT and Laplace-domain time models propose that the brain represents past time as a logarithmically compressed buffer, with time cells implementing the inverse Laplace transform.

## Recent Developments (continued)

- **Human evidence:** Umbach et al. (2020) recorded time cells in human anterior temporal lobe in epilepsy patients during memory tasks, confirming the rodent finding extends to humans.

## Connections

Time cells round out, with [[place-cells|Place Cells]] and [[grid-cells|Grid Cells]], the hippocampal-entorhinal circuit's representation of episodic context: where, when, and (via lateral entorhinal "what" input) which event. They are the temporal scaffold for [[episodic-memory|Episodic Memory]] and are recruited into [[memory-engrams|Memory Engrams]] for time-structured experiences.

For VaultMind, time cells motivate first-class temporal axes in episode storage. Each event has a "when" coordinate, recency-weighted retrieval is the natural readout, and rescaling under different time horizons (last hour vs. last week) is exactly what time cells do biologically.
