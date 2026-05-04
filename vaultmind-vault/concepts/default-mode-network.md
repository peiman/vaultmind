---
id: concept-default-mode-network
type: concept
title: Default Mode Network
created: 2026-04-26
tags:
  - neuroscience
  - resting-state
  - fmri
  - self-referential-thought
related_ids:
  - concept-neural-replay
  - concept-episodic-memory
  - concept-predictive-coding
  - concept-free-energy-principle
source_ids:
  - source-raichle-2001
---

## Overview

The **default mode network (DMN)** is a set of brain regions — most prominently the posterior cingulate cortex / precuneus, medial prefrontal cortex, lateral parietal cortex / angular gyrus, and parts of medial temporal lobe — whose activity *decreases* below resting baseline whenever a person engages in an externally directed, goal-driven task, and *increases* when the person is at rest, daydreaming, mind-wandering, recalling autobiographical events, simulating the future, or reasoning about other minds.

Identified and named by Marcus Raichle and colleagues in 2001 ([[source-raichle-2001|paper]]), the DMN inverted the long-standing assumption that resting brain was a passive baseline. Resting brain is highly active — just active in a task-negative, intrinsic mode that goal-directed tasks suppress.

## How It Works

The DMN is defined functionally rather than anatomically: it is the set of regions exhibiting correlated low-frequency BOLD fluctuations at rest, and consistent task-induced deactivation across many cognitive paradigms. Anatomically, the regions are linked by long-range white-matter tracts (cingulum bundle, superior longitudinal fasciculus) and share a high baseline metabolic rate even relative to other cortical regions.

Functionally, the DMN appears to support *internally directed cognition*: episodic memory retrieval, prospection, theory of mind, narrative comprehension, self-referential evaluation, and mind-wandering. It is anti-correlated at rest with the dorsal attention network and the executive control network, with which it is dynamically reciprocal — when one is up, the other is down.

## Key Findings

- **Task-induced deactivation:** A consistent set of regions deactivates across many cognitive tasks ([[source-raichle-2001|Raichle et al. 2001]]; Shulman et al. 1997).
- **Intrinsic correlation structure:** Even in the absence of any task, DMN regions show correlated spontaneous BOLD fluctuations, identifiable as a functional network (Greicius et al. 2003).
- **Internal cognition:** The DMN is engaged by autobiographical memory, episodic future thinking, theory of mind, and moral judgement — all forms of self- and other-modelling.
- **Mind-wandering:** Mason et al. (2007) showed DMN engagement tracks self-reported mind-wandering, linking the network to the "wandering mind" literature.
- **Energy and metabolism:** The brain spends most of its energy budget on intrinsic (DMN-related) activity rather than stimulus-driven activity (Raichle 2010); evoked responses are tiny perturbations on a large baseline.

## Recent Developments

- **DMN in dementia:** DMN connectivity declines in ageing and is severely disrupted in Alzheimer's disease, with amyloid-β deposition concentrating in DMN hubs. The DMN is, in effect, an anatomical map of where Alzheimer's strikes.
- **DMN and depression:** Hyperconnectivity within the DMN — especially involving rumination-related regions — is a robust correlate of major depression and a target for therapeutic intervention.
- **Psychedelics:** Carhart-Harris et al. (2014, 2016) showed that psilocybin and LSD acutely *decrease* DMN integrity, a state that correlates with self-dissolution experiences and is hypothesised to underlie therapeutic effects in depression.
- **DMN and replay:** Higgins, Liu et al. (2021, *Nature Comms*) showed that DMN activity in resting humans accompanies sequential MEG-detectable replay of recently experienced task structure, plausibly extending hippocampal replay to a brain-wide, DMN-indexed phenomenon.
- **DMN and large-scale gradient organisation:** Margulies et al. (2016) placed DMN at the extreme of the principal cortical gradient — the network most distant in connectivity from sensorimotor cortex, occupying the apex of cortical abstraction.

## Connections

The DMN is the macroscopic cortical correlate of the internally generated cognition that supports [[episodic-memory|Episodic Memory]] (recalling), [[neural-replay|Neural Replay]] (offline reactivation, with which DMN activity is increasingly co-localised), and the generative-model side of [[predictive-coding|Predictive Coding]] / [[free-energy-principle|Free Energy Principle]] (running the model when the world is not constraining it).

For VaultMind, the DMN is the strongest empirical case for treating idle time as computationally first-class. An always-on knowledge agent should have a "default mode" — a between-query process that integrates, simulates, links, and consolidates — rather than dropping to zero CPU when no one is asking it questions.
