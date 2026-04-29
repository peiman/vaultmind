---
id: concept-neural-replay
type: concept
title: Neural Replay
created: 2026-04-26
vm_updated: 2026-04-26
tags:
  - neuroscience
  - hippocampus
  - consolidation
  - sleep
  - planning
related_ids:
  - concept-sharp-wave-ripples
  - concept-place-cells
  - concept-memory-consolidation
  - concept-memory-replay
  - concept-memory-engrams
  - concept-default-mode-network
source_ids:
  - source-wilson-mcnaughton-1994
  - source-buzsaki-2015
---

## Overview

**Neural replay** is the offline reactivation, by the brain itself and without external cues, of patterns of neural activity that previously occurred during waking experience. It was first demonstrated by [[source-wilson-mcnaughton-1994|Wilson & McNaughton (1994)]] as elevated co-firing of co-active hippocampal [[place-cells|place cells]] during sleep that followed running, and is now understood as a brain-wide phenomenon spanning hippocampus, neocortex, ventral striatum, and prefrontal cortex.

Replay is the leading candidate mechanism by which the brain consolidates memories, performs credit assignment, simulates future plans, and generalises across experiences. It is, alongside the [[default-mode-network|Default Mode Network]] and [[sharp-wave-ripples|Sharp-Wave Ripples]], the strongest evidence that "rest" is computationally non-trivial.

## How It Works

In the canonical hippocampal case, replay occurs inside [[sharp-wave-ripples|sharp-wave ripple]] events during quiet wakefulness and non-REM sleep. A sequence of place cells that fired during a 1-2 second waking run (say, A → B → C → D along a track) re-fires in the same order in 100-150 ms — a roughly 10× temporal compression matched to spike-timing-dependent plasticity windows. The compression is computationally important: it is what lets a downstream learner integrate the sequence within a single plasticity window.

Replay flavours include:

- **Forward replay:** The sequence runs in the experienced order. Common during quiet wakefulness *before* a behaviour, suggestive of planning.
- **Reverse replay:** The sequence runs backward. Common immediately after reward, suggestive of credit assignment.
- **Off-track replay:** Sequences corresponding to trajectories the animal never actually took, including shortcuts and remote environments — useful for generalisation and offline planning.

## Key Findings

- **Place-cell replay during sleep:** Foundational finding ([[source-wilson-mcnaughton-1994|Wilson & McNaughton 1994]]).
- **Reverse replay at reward:** Foster & Wilson (2006) showed reverse replay of just-completed runs at reward locations.
- **Awake replay supports planning:** Jadhav et al. (2012) disrupted awake hippocampal SPW-Rs and impaired upcoming choice behaviour, causally implicating replay in planning.
- **Cortical replay:** Replay is not only hippocampal — Euston, Tatsuno & McNaughton (2007) showed compressed replay of cortical task sequences during sleep, on a faster timescale.
- **Striatal and reward replay:** Pennartz et al. (2004) extended replay to ventral striatum, linking it to value learning.
- **Generalisation and inference:** Replay of trajectories never taken supports relational inference and shortcut-finding (Gupta et al. 2010; Liu et al. 2019, *Cell*).

## Recent Developments

- **Replay in humans:** Liu et al. (2019, *Cell*) used MEG to detect rapid sequential reactivation of task representations in resting humans, showing factorised structure-and-content replay analogous to rodents.
- **Replay is selective:** Yang et al. (2024, *Science*) showed that hippocampal SPW-R replay tags which experiences will survive into long-term memory — replay does not simply rehearse everything.
- **Replay in artificial agents:** Experience replay (Lin 1992) is a foundational ingredient of deep RL (DQN, R2D2) and is widely interpreted as an engineered analogue of hippocampal replay; complementary-learning-systems theory (McClelland, McNaughton & O'Reilly 1995) explicitly motivates this connection.
- **Theta sequences as compressed replay during behaviour:** Within each theta cycle (~125 ms), CA1 place cells produce a compressed forward sweep — an "online" mini-replay that may bridge waking experience and offline replay.

## Connections

Neural replay is carried by [[sharp-wave-ripples|Sharp-Wave Ripples]], reactivates [[place-cells|Place Cell]] (and time-cell, and engram) sequences, and is the operational mechanism by which [[memory-consolidation|Memory Consolidation]] transfers information from hippocampus to cortex. It overlaps in time with [[default-mode-network|Default Mode Network]] activity during quiet rest and is the substrate-level event behind [[memory-replay|Memory Replay]] more broadly.

For VaultMind, replay is the canonical biological argument that an agent's idle time is computationally productive. Experience replay in deep RL is the closest engineering analogue. A VaultMind-equipped agent should treat between-query intervals as windows for offline re-indexing, link inference, and self-distillation — not as wasted cycles.
