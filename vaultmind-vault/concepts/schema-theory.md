---
id: concept-schema-theory
type: concept
title: Schema Theory
created: 2026-04-07
vm_updated: 2026-04-07
aliases:
  - Bartlett's Schema Theory
  - Constructive Memory
tags:
  - cognitive-science
  - encoding
  - memory-organization
related_ids:
  - concept-semantic-networks
  - concept-associative-memory
  - concept-episodic-memory
source_ids:
  - source-bartlett-1932
---

## Overview

Schema theory holds that human memory is not a passive recording system but an active, constructive process shaped by prior knowledge structures called schemas. The theory was developed by Frederic Bartlett in his 1932 book *Remembering*, based on a series of experiments in which British participants recalled an unfamiliar Native American folk tale ("The War of the Ghosts") across multiple retelling sessions.

Bartlett found that participants systematically distorted the story over time: details that didn't fit British cultural expectations were dropped, rationalized, or replaced with culturally familiar substitutes. The story became shorter, more coherent by British norms, and increasingly anglicized with each retelling. This led Bartlett to argue that remembering is not reproduction but *reconstruction* — the memory system fills in gaps and resolves ambiguities using existing knowledge frameworks (schemas).

A schema is a mental framework that organizes and interprets information within a domain — a "restaurant schema" encodes the expected sequence (enter, order, eat, pay) so that recalling a specific restaurant visit can invoke the schema to fill in unremembered details. Schemas allow efficient encoding and retrieval of typical events, but at the cost of distorting the atypical: schema-inconsistent information may be dropped (if it resists assimilation) or exaggerated (if it is so distinctive it forms a standalone memory).

## Key Properties

- **Reconstructive memory:** Recall actively rebuilds episodes from fragments plus schema-consistent inferences — memory is not a playback but a re-creation
- **Schema-driven distortion:** Information inconsistent with existing schemas is at risk of being dropped, altered, or rationalized toward schema-consistent alternatives
- **Schema-consistent advantage for typical events:** Expected, schema-consistent information is processed more efficiently and integrated more smoothly into memory
- **Schema-inconsistent advantage for distinctive items:** Highly unexpected items can be better remembered precisely because they violate expectations (the "von Restorff effect" applied to schema incongruity)
- **Schemas evolve:** Schemas are not static; new experiences that fail to assimilate can trigger accommodation — modifying the schema itself

## Connections

VaultMind's note type schema (concept, source, person, decision, project) functions as a cognitive schema for agent knowledge: it constrains what kinds of information are captured, which fields are required, and how notes relate to one another. Just as Bartlett's participants reconstructed stories through a British cultural schema, an agent using VaultMind retrieves and interprets knowledge through the lens of the vault's type hierarchy.

The reconstructive nature of schema-guided memory is relevant to how VaultMind handles incomplete notes. A note with sparse frontmatter is a memory fragment; retrieval via [[context-pack|Context Pack]] supplements it with neighboring graph context, effectively performing schema-guided reconstruction — filling in what is not explicitly present from what is structurally implied by the surrounding [[semantic-networks|Semantic Networks]] graph.
