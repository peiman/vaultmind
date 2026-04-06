---
id: concept-interference-theory
type: concept
title: Interference Theory
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Proactive Interference
  - Retroactive Interference
tags:
  - cognitive-science
  - forgetting
  - retrieval
related_ids:
  - concept-forgetting-curve
  - concept-spacing-effect
  - concept-encoding-specificity
source_ids: []
---

## Overview

Interference theory is a classical cognitive science account of forgetting that attributes memory failure to competition between stored memories rather than to passive decay. The core claim: memories are not simply lost over time; they are suppressed or overwritten by other, competing memories. The more similar the competing memories are, the stronger the interference.

Two forms are distinguished. **Proactive interference** occurs when older memories impair the learning or retrieval of new material — prior knowledge "gets in the way" of encoding something new. **Retroactive interference** occurs when newly learned material impairs recall of older memories — new knowledge overwrites or obscures what was previously stored. Both forms are most potent when the competing memories share similar cues, contexts, or content.

Interference theory emerged in the early 20th century as an alternative to pure decay accounts (see [[forgetting-curve|Forgetting Curve]]), and remains influential in memory research alongside encoding-based accounts (see [[encoding-specificity|Encoding Specificity]]). The [[spacing-effect|Spacing Effect]] partially counteracts interference by distributing practice across time, reducing the similarity between successive learning episodes.

## Key Properties

- **Competition-based forgetting:** Memories fail because competing traces win the retrieval race, not simply because time has passed
- **Similarity amplifies interference:** Highly similar memories (same topic, same cues) interfere more than dissimilar ones
- **Proactive interference:** Old → new disruption; prior learning impairs acquisition of new material
- **Retroactive interference:** New → old disruption; recent learning impairs recall of older material
- **Cue dependency:** Interference is strongest when competing memories share retrieval cues — accessing one strongly-associated item inhibits access to others
- **Fan effect:** As more items become associated with a single cue, retrieval time and error rate increase (a direct consequence of interference)

## Connections

When a VaultMind vault contains many similar notes on a topic — multiple notes about the same concept, person, or project — retrieval must handle the cognitive-science analog of interference: multiple notes competing for the same query embedding or graph traversal path. This is not merely a ranking problem; it is structurally the same as the fan effect in associative memory.

VaultMind's confidence-weighted edges and priority ordering in [[context-pack|Context Pack]] assembly function as interference resolution mechanisms. By giving higher confidence to explicit relations over inferred ones, and by capping the number of retrieved neighbors, the system reduces the "fan" of competing candidates that the consuming agent must reason over.

The entity resolution subsystem is also relevant here: when two notes refer to the same real-world entity under different names, they create proactive/retroactive interference in the agent's reasoning — the agent may reconcile them, confuse them, or treat them as distinct when they are not. Confident entity resolution reduces this interference before it reaches the agent.

Future VaultMind versions could model interference explicitly: if many notes share a concept tag or graph neighborhood, their individual retrieval confidence should be discounted relative to vaults where retrieved notes are more distinctive. This would mirror the fan effect penalty in [[act-r|ACT-R]]-style memory models.
