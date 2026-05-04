---
id: concept-predictive-coding
type: concept
title: Predictive Coding
created: 2026-04-26
tags:
  - neuroscience
  - computational-neuroscience
  - hierarchical-models
  - inference
related_ids:
  - concept-free-energy-principle
  - concept-pyramidal-neurons
  - concept-interneurons
  - concept-default-mode-network
  - concept-hebbian-learning
source_ids:
  - source-rao-ballard-1999
  - source-friston-2010
---

## Overview

**Predictive coding** is the hypothesis that the brain is fundamentally a hierarchical prediction machine. Each level of cortex maintains an internal generative model that tries to predict the activity of the level below it via top-down (feedback) connections. Bottom-up (feedforward) connections do not transmit raw sensory data — they transmit only the *residual error* between the prediction and the actual lower-level activity. Perception, on this view, is the brain's inference about the hidden causes of its sensory input, refined by minimising prediction error across the cortical hierarchy.

The modern neural-coding form of the theory was formalised by [[source-rao-ballard-1999|Rao & Ballard (1999)]] for visual cortex, building on earlier ideas from Helmholtz ("unconscious inference"), Mumford (1992), and information-theoretic accounts of efficient sensory coding. It was later embedded in the broader [[free-energy-principle|Free Energy Principle]] framework by Karl Friston ([[source-friston-2010|2010]]).

## How It Works

In a canonical predictive-coding circuit, each cortical level contains two functional populations:

- **Representation units** that encode the current best estimate of the latent variable at that level.
- **Error units** that encode the difference between the top-down prediction (sent down from the level above) and the actual representation activity at the current level.

Feedback connections from higher levels carry predictions; feedforward connections carry prediction errors. Learning adjusts the weights so that, on average, predictions match incoming activity — minimising error. The bias between deep and superficial cortical layers, and between glutamatergic and GABAergic populations, plausibly maps onto this representation/error split, although the cell-type-level identification is still debated.

A key consequence is that *if* a higher level's prediction perfectly explains lower-level activity, no error signal propagates upward — silence rather than activity is the signature of a successful prediction. This explains a wide range of "extra-classical" effects in visual cortex such as endstopping and surround suppression, and motivates "explaining away" effects across cortical hierarchies.

## Key Findings

- **Hierarchical visual model reproduces extra-classical effects:** [[source-rao-ballard-1999|Rao & Ballard (1999)]] trained their model on natural images and obtained simple-cell-like receptive fields plus endstopping — a feedforward-only model could not.
- **Mismatch responses:** Whenever sensory input deviates from expectation (oddball tones, omitted notes in a sequence, illegal transitions), large error signals appear in EEG / MEG (mismatch negativity, P300) and in single-unit recordings in primary cortex — strong evidence the brain computes prediction errors.
- **Top-down attenuation:** Predictable stimuli evoke smaller cortical responses than identical but unpredicted stimuli — repetition suppression and expectation suppression are predictive-coding signatures.
- **Active inference:** Predictive coding generalises to action: rather than only changing predictions to match input, the agent can also act so that input matches predictions ("I will perceive my hand at position X" → motor commands move the hand to X).

## Recent Developments

- **Canonical microcircuit models:** Bastos, Friston, Adams and colleagues have proposed laminar-specific predictive-coding microcircuits, with deep pyramidal cells projecting predictions and superficial cells transmitting errors.
- **Predictive coding in ML:** The deep predictive-coding network (PredNet, Lotter et al. 2017) and self-supervised "predict the next token / patch / frame" objectives are widely seen as engineering instantiations of the theory. The sudden empirical success of next-token prediction has reinvigorated the predictive-coding literature.
- **Limits and critiques:** Direct cell-type-level evidence for separate "prediction" and "error" populations remains contested (Mehrer et al. 2020; Kogo & Trengove 2015). Some find that predictive coding is too flexible — it accommodates data after the fact more than it constrains it.
- **Predictive processing and psychiatry:** Aberrant precision weighting of predictions vs. errors has been proposed as a unifying account of schizophrenia (Sterzer et al. 2018) and autism (Van de Cruys et al. 2014).

## Connections

Predictive coding is the cortical-coding-level companion to the broader [[free-energy-principle|Free Energy Principle]]. It depends on Hebbian-like plasticity ([[hebbian-learning|Hebbian Learning]]) for learning the generative model, on cell-type-specific roles for [[pyramidal-neurons|pyramidal neurons]] and [[interneurons|interneurons]], and is hypothesised to operate in [[default-mode-network|Default Mode Network]] regions during simulation and self-modelling.

For VaultMind, predictive coding suggests that retrieval should be prediction-error-driven. A note that confirms what the agent already believes carries little information; a note whose content would meaningfully change the agent's current model is the one worth fetching. A "surprise threshold" below which retrieval is suppressed is the natural design pattern.
