---
id: concept-utah-array
type: concept
title: Utah Intracortical Electrode Array
created: 2026-04-29
vm_updated: 2026-04-29
tags:
  - bci
  - hardware
  - neuroscience
related_ids:
  - concept-brain-computer-interface
  - concept-neuralink
  - concept-population-decoding
  - concept-closed-loop-neural-control
  - concept-action-potential
  - concept-pyramidal-neurons
  - concept-neuron
source_ids:
  - source-maynard-1997
  - source-hochberg-2006
  - source-pandarinath-2017
  - source-wikipedia-utah-array
---

## Overview

The Utah intracortical electrode array (UIEA) is the silicon-needle microelectrode array originally developed by Richard Normann's group at the University of Utah and commercialized by Cyberkinetics, later Blackrock Neurotech. It is the workhorse intracortical [[brain-computer-interface|BCI]] sensor: the device that has produced essentially every major human intracortical BCI result of the last twenty years, including BrainGate's cursor and robotic-arm work and the high-performance typing demonstrations of the late 2010s.

## How It Works

**Geometry.**
- 4 × 4 mm silicon substrate.
- 100 conductive needles arranged in a 10 × 10 grid (96 typically active in clinical use).
- Needle length 1.0-1.5 mm; intended to land electrode tips in cortical layer 4/5.

**Electrochemistry.**
- Insulating glass / polyimide along each shaft.
- Iridium oxide (IrOx) coating at the tips, giving low impedance, high charge-injection capacity, and long-term stability — the iridium oxide tip is what makes a Utah array a Utah array rather than a generic silicon multi-needle probe.

**Signal.**
- Records [[action-potential|action potentials]] from small populations of [[neuron|neurons]] near each tip — typically 1-3 sortable single units per channel in motor cortex of [[pyramidal-neurons|pyramidal cells]].
- Signal-to-noise ratio reported by Maynard et al. (1997): ~6:1 average.
- Information rate is high enough for real-time BCI decoding via [[population-decoding|population decoding]] methods.

**Implantation.**
- Inserted with a pneumatic impactor in a single ~200 µs stroke; the speed minimizes tissue dimpling and vasculature damage.
- Held in place by a thin polyimide wire bundle to a percutaneous pedestal.

## History

- **[[source-maynard-1997|Maynard, Nordhausen & Normann (1997)]]** — characterized the array as a candidate BCI sensor; the founding paper.
- **2004 onward** — Cyberkinetics' BrainGate trial; [[source-hochberg-2006|Hochberg et al. (2006)]] reported the first human cursor and prosthetic-arm control.
- **2012-2017** — BrainGate2 results: high-DOF arm control (Hochberg 2012), stable decoding over years (Simeral 2011, Pandarinath 2017), high-performance typing ([[source-pandarinath-2017|Pandarinath 2017]]: 24-39 cpm).
- **2020s** — speech BCI demonstrations (UCSF, Stanford BrainGate2) using Utah arrays in motor and speech cortex achieving 60-80 wpm.

## Limitations

The array is the canonical intracortical BCI for a reason — but it has well-known limits that motivate every successor design including [[neuralink|Neuralink]]'s threads:

- **Stiffness mismatch.** Silicon (~150 GPa) versus brain tissue (~1 kPa) drives chronic micromotion and gliosis; a substantial fraction of channels lose signal over months to years.
- **Channel count.** 96 channels is small relative to what a population decoder can use; modern flexible-probe designs target 1000-10000+.
- **Surgical workflow.** The percutaneous pedestal is infection-prone and incompatible with home use.
- **Coverage.** Penetrates only ~1.5 mm; cannot reach deep targets or sample over a wide cortical area.

## Connections

The Utah array is the embodiment of intracortical [[brain-computer-interface|BCI]] up to the present moment; [[neuralink|Neuralink]]'s threads are the most prominent attempt to succeed it. Its outputs feed [[population-decoding|population decoders]] (Kalman / RNN / FORCE) that produce control signals for closed-loop systems ([[closed-loop-neural-control|closed-loop neural control]]).
