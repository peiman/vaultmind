---
id: concept-neuralink
type: concept
title: Neuralink
created: 2026-04-29
tags:
  - bci
  - neuralink
  - hardware
  - clinical
related_ids:
  - concept-brain-computer-interface
  - concept-utah-array
  - concept-population-decoding
  - concept-closed-loop-neural-control
source_ids:
  - source-musk-neuralink-2019
  - source-wikipedia-neuralink
---

## Overview

Neuralink is a US neurotechnology company, founded in 2016 and led by Elon Musk, developing a high-channel-count intracortical [[brain-computer-interface|BCI]] platform. Its design choices stake out a deliberate contrast with the rigid silicon [[utah-array|Utah array]] that has defined intracortical BCIs for thirty years: thousands of fine polymer "threads" instead of hundreds of stiff needles, robotic insertion instead of pneumatic pistol, and an integrated sealed implant instead of a transcutaneous pedestal. The first public technical description was the 2019 [[source-musk-neuralink-2019|JMIR whitepaper]] (Musk & Neuralink); the first human implant came in January 2024.

## How It Works

**Threads (electrodes).**
- Flexible polymer probes carrying microscale gold/PEDOT electrode sites.
- 2019 platform: up to 3072 electrodes on 96 threads (~32 sites per thread).
- Threads are far thinner than Utah needles, intended to reduce mechanical mismatch with brain tissue and dampen the chronic foreign-body response that causes Utah arrays to lose channels over months/years.

**Surgical robot.**
- A custom robot inserts threads one at a time using a fine needle, with micron-scale targeting that avoids surface vasculature visible under intraoperative imaging.
- Quoted insertion rate: six threads (192 electrodes) per minute.
- The procedure is intended to be standardizable rather than artisanal — a core piece of Neuralink's clinical-deployment thesis.

**Implant ("N1" / "Telepathy").**
- A coin-sized, hermetically sealed module containing custom amplifier and digitizer ASICs, packaged for a 23×18.5×2 mm footprint at 3072 channels.
- Wireless inductive power and data link; no transcutaneous connector.

## Recent Developments

- **May 2023** — FDA Investigational Device Exemption granted for human clinical trials.
- **January 29, 2024** — First human implant: Noland Arbaugh, a 29-year-old quadriplegic. Subsequently demonstrated cursor control, web browsing, chess, and game play.
- **2024-2025** — Reports of thread retraction reducing functional channel count in the first participant; mitigated in software via re-decoding.
- **Late 2025** — Twelve trial participants reported, with ~2,000 cumulative days and ~15,000 hours of usage; first UK implant October 28, 2025.
- **Vision program** — separate "Blindsight" program targeting visual cortex stimulation entered preclinical and early human stages.

The company has been controversial: animal-research disclosures (USDA / DOT investigations 2022-2023), opacity around its early clinical trial, and a long-running debate over whether its performance differential vs BrainGate justifies the surgical risk. Independent comparison against the [[source-pandarinath-2017|Pandarinath 2017]] BrainGate baseline remains thin in the public record as of early 2026.

## Connections

The conceptual neighbour is the [[utah-array|Utah array]] — Neuralink's threads are explicitly designed against its limitations. Sits inside the larger [[brain-computer-interface|BCI]] story. Decoding is a [[population-decoding|population decoding]] problem on the recorded thread channels; future stimulating versions belong to [[closed-loop-neural-control|closed-loop neural control]].
