---
id: concept-glymphatic-system
type: concept
title: Glymphatic System
created: 2026-04-26
tags:
  - neuroscience
  - waste-clearance
  - sleep
  - csf
  - alzheimers
related_ids:
  - concept-neuron
  - concept-memory-consolidation
  - concept-default-mode-network
source_ids:
  - source-iliff-2012
  - source-xie-2013
---

## Overview

The **glymphatic system** is the brain's recently discovered macroscopic waste-clearance pathway: cerebrospinal fluid (CSF) flows into the brain along paravascular spaces around penetrating arteries, mixes with interstitial fluid via aquaporin-4 (AQP4) channels on astrocytic endfeet, then drains out along veins, carrying soluble waste products — including amyloid-β, tau, and lactate — to be cleared from the brain. The name combines "glia" (astrocytes are the active component) with "lymphatic" (the function it provides).

Maiken Nedergaard's lab described the system in mice in 2012 ([[source-iliff-2012|Iliff et al.]]) and showed in 2013 ([[source-xie-2013|Xie et al.]]) that its throughput increases dramatically during sleep — providing a long-sought concrete answer to "what is sleep for, biologically?".

## How It Works

CSF produced in the choroid plexus flows out through the ventricles and over the brain surface, then enters the brain parenchyma along the perivascular (Virchow-Robin) spaces surrounding penetrating arteries. AQP4 water channels concentrated on astrocytic endfeet that line these spaces drive bulk water flow from the perivascular CSF into the interstitial space. Interstitial fluid, now mixed with CSF, drains along paravenous spaces, eventually leaving the brain via meningeal lymphatics (Louveau et al. 2015; Aspelund et al. 2015 — the brain *does* have lymphatics after all).

The interstitial space itself is the limiting factor: in awake brain it is packed (~14% of brain volume); during sleep or anaesthesia it expands to ~23%, increasing convective fluxes by roughly an order of magnitude.

## Key Findings

- **Paravascular CSF-ISF exchange:** Two-photon tracer imaging directly visualises CSF flowing into the parenchyma along arteries and out along veins ([[source-iliff-2012|Iliff et al. 2012]]).
- **AQP4 dependence:** AQP4 knockout mice show ~70% reduced clearance of amyloid-β ([[source-iliff-2012|Iliff et al. 2012]]).
- **Sleep gates clearance:** Interstitial space expands by ~60% in sleeping or anaesthetised mice; β-amyloid clearance roughly doubles ([[source-xie-2013|Xie et al. 2013]]).
- **Posture matters:** Glymphatic flow is most efficient in lateral decubitus sleep posture (Lee et al. 2015), the typical mammalian sleep position.
- **Meningeal lymphatics:** Louveau et al. (2015) and Aspelund et al. (2015) discovered functional lymphatic vessels in the dural sinuses, providing the anatomical drainage route from the meninges to deep cervical lymph nodes.

## Recent Developments

- **Human evidence:** Eide et al. (2020, *Brain*) imaged glymphatic flow in living humans using intrathecal gadolinium MRI; flow patterns and clearance rates broadly matched rodent findings.
- **Glymphatic dysfunction in disease:** Reduced glymphatic clearance has been linked to Alzheimer's, traumatic brain injury, normal-pressure hydrocephalus, idiopathic intracranial hypertension, and small-vessel disease. The amyloid hypothesis of Alzheimer's now explicitly includes a clearance-failure axis.
- **Controversies:** A subset of the field (Smith et al. 2017; Asgari et al. 2016) argues that bulk convective flow is implausible on hydrodynamic grounds and that diffusion plus weak pulsation accounts for most observed clearance. The active dispute is over the *mechanism* of glymphatic flow, less so over its existence.
- **Slow-wave sleep specifically:** Subsequent work pinpointed slow-wave (NREM) sleep as the primary clearance state, linking glymphatic function to the same sleep stage that hosts hippocampal replay and [[sharp-wave-ripples|sharp-wave ripples]].
- **Lifestyle modulators:** Exercise and ω-3 intake increase glymphatic flow in rodents; chronic sleep deprivation, hypertension, and ageing decrease it.

## Connections

The glymphatic system links sleep to memory at a different level from [[neural-replay|Neural Replay]] and [[memory-consolidation|Memory Consolidation]]: replay handles the information; the glymphatic system handles the chemistry. Both depend on slow-wave sleep, and both fail together in dementia. Glymphatic dysfunction also intersects with [[default-mode-network|Default Mode Network]] vulnerability — DMN hubs are where amyloid concentrates, plausibly because they are both metabolically active and clearance-bottlenecked.

For VaultMind, the glymphatic system is one of the cleanest biological cases of "the substrate must take time off to clean up". Index compaction, dedup, link cleanup, and stale-note pruning are not afterthoughts but first-class scheduled events that should run during otherwise idle windows.
