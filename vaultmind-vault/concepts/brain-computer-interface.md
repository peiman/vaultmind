---
id: concept-brain-computer-interface
type: concept
title: Brain-Computer Interface (BCI)
created: 2026-04-29
tags:
  - bci
  - neuroscience
  - clinical
related_ids:
  - concept-utah-array
  - concept-neuralink
  - concept-closed-loop-neural-control
  - concept-population-decoding
  - concept-action-potential
  - concept-pyramidal-neurons
  - concept-neuron
source_ids:
  - source-wikipedia-bci
  - source-hochberg-2006
  - source-pandarinath-2017
  - source-musk-neuralink-2019
---

## Overview

A brain-computer interface (BCI) is a direct communication channel between the nervous system and an external effector — a cursor, prosthetic limb, speech synthesizer, or stimulator — that bypasses the normal motor and sensory periphery. BCIs read neural signals (and in closed-loop systems, also write to neural tissue), decode them into commands, and deliver those commands at human-relevant latency. The term was coined by Jacques Vidal in 1973; the modality space has since exploded along three axes — invasiveness, signal type, and direction of information flow.

## Modalities

**Non-invasive (scalp / external):**
- **EEG** — millisecond temporal resolution but centimeter spatial resolution, dominated by population-averaged rhythms.
- **MEG / fMRI / fNIRS** — typically research instruments, not deployable BCIs.
- **Common BCI signals:** P300 evoked potentials, motor imagery mu/beta desynchronization, steady-state visual evoked potentials (SSVEP).

**Partially invasive (under the skull, on cortex):**
- **ECoG / sEEG** — sub-dural electrode grids or stereotactic depth electrodes. Higher SNR than EEG and access to broadband gamma; used in epilepsy patients and increasingly for speech BCIs.

**Invasive intracortical (in cortex):**
- **Microelectrode arrays** — penetrating probes that record from individual [[neuron|neurons]] (single-unit) and small populations. The [[utah-array|Utah array]] is the workhorse; Michigan-style planar probes and Neuropixels are alternatives. [[neuralink|Neuralink]]'s flexible threads sit in this class.
- **Records [[action-potential|action potentials]] directly**, giving the highest information rate per channel but at the cost of surgery and finite chronic-recording lifetime.

**Stimulating (writing into the brain):**
- **DBS** — deep brain stimulation for Parkinson's, essential tremor, OCD.
- **Cortical / spinal microstimulation** — for sensory feedback and reanimation of paralyzed limbs.
- **Vagus / spinal stimulators** — for epilepsy, depression, chronic pain.

## Decoding Pipeline

1. **Acquisition.** Amplify and digitize raw signals (μV-mV range).
2. **Spike sorting / feature extraction.** Detect threshold crossings, sort waveforms by neuron identity, or extract band-power features.
3. **Decoding.** Map neural features to intended output via a [[population-decoding|population decoder]] — Kalman filters, recurrent neural nets, point-process models, or speech-specific phoneme decoders.
4. **Output.** Cursor velocity, joint angles, audible speech, keyboard click, FES current. Feedback (visual, auditory, sometimes intracortical microstimulation) closes the loop.

## Recent Developments

- **BrainGate clinical line.** [[source-hochberg-2006|Hochberg et al. (2006)]] demonstrated cursor and robotic-arm control from a [[utah-array|Utah array]] in a tetraplegic participant. [[source-pandarinath-2017|Pandarinath et al. (2017)]] reached ~24-39 correct characters / minute on free-paragraph typing.
- **Speech BCIs (UCSF, Stanford).** Decoding spoken or attempted speech from cortical activity at ~60-80 wpm in 2023-2025 trials.
- **[[neuralink|Neuralink]].** Flexible thread arrays with thousands of channels, robotic insertion, first human implant January 2024 (Noland Arbaugh), 12+ participants by late 2025.
- **Endovascular BCIs (Stentrode / Synchron).** Stent-mounted electrodes deployed via the jugular vein; first US human trials approved 2021.

## Connections

The intracortical BCI sub-stack rests on the [[utah-array|Utah array]] and similar microelectrode arrays; [[neuralink|Neuralink]] is the most prominent recent attempt to replace those arrays with thread-style probes. Decoding is the [[population-decoding|population decoding]] problem. Stimulation-side BCIs and adaptive neuromodulation belong to [[closed-loop-neural-control|closed-loop neural control]]. At the cellular level the recordable signal is the [[action-potential|action potential]] — predominantly from [[pyramidal-neurons|pyramidal neurons]] in motor and language cortex.
