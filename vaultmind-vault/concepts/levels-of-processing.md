---
id: concept-levels-of-processing
type: concept
title: Levels of Processing
created: 2026-04-07
vm_updated: 2026-04-07
aliases:
  - Depth of Processing
  - Craik-Lockhart Framework
tags:
  - cognitive-science
  - encoding
  - retrieval
related_ids:
  - concept-encoding-specificity
  - concept-semantic-memory
  - concept-forgetting-curve
source_ids:
  - source-craik-lockhart-1972
---

## Overview

Levels of processing (LOP), proposed by Fergus Craik and Robert Lockhart in 1972, is a framework for understanding memory encoding that argues that memory retention depends on the *depth* of processing during encoding rather than on the time spent or the specific memory store used. It was proposed as a direct alternative to the stage-based [[multi-store-model|Multi-Store Model]] of Atkinson and Shiffrin.

Craik and Lockhart identified a continuum from shallow to deep processing. Shallow (structural) processing attends to physical features: "Is this word printed in capital letters?" Intermediate (phonemic) processing attends to sound: "Does this word rhyme with 'train'?" Deep (semantic) processing attends to meaning: "Does this word fit in the sentence 'The child played with the ___'?" Across dozens of experiments, semantic processing produced dramatically superior recall and recognition compared to structural or phonemic processing, even when time on task was equated.

The framework was extended by Craik and Tulving (1975), who showed that elaboration within a processing level also matters: a more elaborate semantic encoding (placing the word in a complex, vivid sentence context) produced better retention than a sparse semantic encoding. This led to the concept of *elaborative rehearsal* as distinct from mere *maintenance rehearsal* — repeating a phone number to hold it in mind improves short-term retention without building durable long-term memory.

## Key Properties

- **Depth continuum:** Structural → phonemic → semantic processing, with deeper processing producing stronger, more durable memory traces
- **Elaboration:** Within a depth level, more elaborate, distinctive encodings produce better retention than sparse ones
- **Maintenance vs. elaborative rehearsal:** Rote repetition holds information in working memory but does not transfer it to durable long-term storage; elaborative rehearsal that connects new information to existing knowledge does
- **Transfer-appropriate processing:** Later work showed that shallow processing can outperform deep processing when the retrieval test also requires shallow processing — the advantage of depth is not absolute, but depends on match between encoding and retrieval conditions
- **No discrete stages:** LOP rejected fixed sensory registers and short-term stores in favor of a single, continuous processing dimension

## Connections

Levels of processing has a direct implication for how VaultMind notes should be authored. A note that merely records a raw excerpt (structural/shallow encoding) will be less retrievable and less useful than a note that explains the concept's meaning, situates it in the graph via `related_ids`, and elaborates with examples (semantic/deep encoding). VaultMind's graph structure rewards deep encoding: richly cross-linked notes with semantic annotations participate in more retrieval paths than sparse, isolated notes.

This connects to [[encoding-specificity|Encoding Specificity]] — deep processing tends to produce more distinctive memory traces with more potential retrieval cues. The [[spacing-effect|Spacing Effect]] similarly benefits from elaborative rather than maintenance rehearsal: spacing works best when each review involves active re-processing of meaning, not passive re-reading.
