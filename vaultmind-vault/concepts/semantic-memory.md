---
id: concept-semantic-memory
type: concept
title: Semantic Memory
created: 2026-04-07
vm_updated: 2026-04-07
aliases:
  - Factual Memory
  - Knowledge Memory
tags:
  - cognitive-science
  - memory-systems
  - tulving
related_ids:
  - concept-episodic-memory
  - concept-semantic-networks
  - concept-associative-memory
source_ids:
  - source-tulving-1972
---

## Overview

Semantic memory is the cognitive system that stores general world knowledge — facts, meanings, concepts, categories, and their relationships — independently of any specific personal episode. The term was introduced by Endel Tulving in 1972 alongside [[episodic-memory|Episodic Memory]] as part of his two-system taxonomy of declarative long-term memory.

Where episodic memory encodes *personally experienced* events with temporal and spatial context, semantic memory holds decontextualized knowledge: the capital of France, the meaning of "photosynthesis," how to use a calculator. You know these facts without remembering when or how you learned them. Tulving characterized this as "noetic" consciousness — knowing something without re-experiencing the moment of acquisition.

Semantic memory is remarkably robust compared to episodic memory. Patients with anterograde amnesia who cannot form new episodic memories can still retain semantic knowledge accumulated before their injury, and can even acquire new semantic facts through sufficient repetition — though without any episodic record of having learned them. This dissociation provides strong evidence for the separability of the two systems.

## Key Properties

- **Noetic consciousness:** Retrieval involves knowing, not re-experiencing — facts are accessed without mental time travel
- **Context-free encoding:** Knowledge is stored without the temporal, spatial, or emotional context of the learning episode
- **Organized by meaning:** Semantic memory is structured by conceptual relationships (categories, properties, hierarchies) rather than by temporal order
- **Highly stable:** Less susceptible to distortion than episodic memory; facts resist the reconstructive errors that plague autobiographical recall
- **Dissociable from episodic:** Hippocampal damage impairs episodic memory far more than semantic memory, which depends more on lateral temporal cortex

## Connections

The VaultMind vault functions as an agent's semantic memory store. When an agent indexes a concept note, it detaches the knowledge from the episodic context of its acquisition and makes it available as a general, queryable fact. The `related_ids` graph encodes semantic relationships between concepts — analogous to how semantic memory organizes knowledge by meaning rather than by episode.

The [[semantic-networks|Semantic Networks]] literature directly models semantic memory as a typed property graph, which is the same structure VaultMind uses for its note graph. VaultMind's entity resolution and type schema impose the categorical organization that is the hallmark of semantic memory — concepts, sources, people, and decisions are distinct semantic categories with distinct inference rules.
