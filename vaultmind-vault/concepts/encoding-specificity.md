---
id: concept-encoding-specificity
type: concept
title: Encoding Specificity
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Encoding Specificity Principle
  - Context-Dependent Memory
tags:
  - cognitive-science
  - retrieval
  - encoding
related_ids:
  - concept-associative-memory
  - concept-spreading-activation
source_ids:
  - source-tulving-thomson-1973
---

## Overview

The encoding specificity principle, formulated by Endel Tulving and Donald Thomson (1973), states that memory retrieval is most successful when the conditions at retrieval match the conditions present during encoding. The cues available at recall must overlap with the information encoded alongside the target memory.

This principle explains why studying in the same room where you'll take the test improves recall, why mood-congruent memory occurs, and why recognition can sometimes fail when recall succeeds (if the recognition context doesn't match encoding).

## Key Properties

- **Context reinstatement:** Returning to the original encoding context improves retrieval
- **Cue-dependent forgetting:** Information isn't lost — it's inaccessible because the right retrieval cues aren't present
- **Outshining:** Strong cues can override weak contextual matches, but weak cues rely heavily on context
- **Transfer-appropriate processing:** Memory is best when the type of processing at retrieval matches encoding (semantic encoding → semantic cues)

## Connections

VaultMind's retrieval is context-blind — the same query always produces the same result regardless of what the agent is currently working on. The expert panel (Session 02, Vasquez) identified this as a gap: [[context-pack|Context Pack]] assembly has no notion of the agent's active context or current task. A future contextual query mode for `memory related` could accept an active context (a set of note IDs currently "in focus") to bias retrieval, implementing a minimal form of encoding specificity.
