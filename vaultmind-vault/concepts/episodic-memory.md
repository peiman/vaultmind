---
id: concept-episodic-memory
type: concept
title: Episodic Memory
created: 2026-04-07
vm_updated: 2026-04-07
aliases:
  - Autobiographical Memory
  - Event Memory
tags:
  - cognitive-science
  - memory-systems
  - tulving
related_ids:
  - concept-semantic-memory
  - concept-encoding-specificity
  - concept-working-memory
source_ids:
  - source-tulving-1972
---

## Overview

Episodic memory is the memory system dedicated to personally experienced events — the mental records of what happened, where it happened, and when it happened. The term was introduced by Endel Tulving in 1972 to distinguish this experiential form of memory from semantic memory (general knowledge). Tulving described episodic memory as "autonoetic" — the act of retrieval involves mentally traveling back in time and re-experiencing the event from a first-person perspective, rather than simply knowing a fact.

Episodic memory is uniquely human in its full form. It is the system that allows you to recall your first day at a new job, reconstruct the sequence of events in a conversation, or remember that you already checked your email this morning. Because it encodes context (time, place, emotional state, surrounding events), it is highly cue-dependent: retrieval succeeds when the conditions at recall overlap with those at encoding.

Episodic memory is also reconstructive rather than reproductive. Each act of retrieval rebuilds the episode from stored fragments and schema-consistent inferences, which makes it both flexible and error-prone. Distortions such as source confusion, post-event misinformation, and false memories arise precisely because episodic recall is an act of construction.

## Key Properties

- **Autonoetic consciousness:** Retrieval involves mental time travel — subjective re-experiencing of the past, not merely knowing it
- **What-where-when encoding:** Episodes are indexed by content, spatial context, and temporal position
- **Cue-dependence:** Episodic memories are most accessible when retrieval cues overlap with the encoding context (see [[encoding-specificity|Encoding Specificity]])
- **Reconstructive nature:** Episodes are rebuilt from fragments at retrieval, making them susceptible to distortion and interference
- **Hippocampal dependence:** Episodic memory critically depends on hippocampal binding — patients with hippocampal lesions (like H.M.) cannot form new episodic memories

## Connections

VaultMind notes carry timestamps (`created`, `vm_updated`) and originate from specific agent sessions, making them approximate episodic traces — they record not just *what* was known but *when* and in what operational context it was captured. The `source_ids` field functions like episodic context, grounding a concept note in the particular text or interaction that produced it.

This stands in contrast to the vault's role as a [[semantic-memory|Semantic Memory]] store: individual notes are episodic artifacts, but the indexed, cross-linked graph of the vault as a whole functions semantically. The distinction matters for retrieval: temporal queries ("what did the agent learn during the project kickoff?") are episodic; topical queries ("what does the vault know about token budgets?") are semantic.
