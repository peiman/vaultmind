---
id: concept-cognitive-load-theory
type: concept
title: Cognitive Load Theory
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Sweller's CLT
  - Cognitive Load
tags:
  - cognitive-science
  - learning
  - working-memory
related_ids:
  - concept-working-memory
  - concept-schema-theory
  - concept-multi-store-model
source_ids:
  - source-sweller-1988
---

## Overview

Cognitive Load Theory, introduced by John Sweller (1988), proposes that effective learning depends on managing the demands placed on working memory during instruction. Because working memory has severely limited capacity — approximately 4 chunks (Cowan 2001) — instruction design must actively manage how that capacity is allocated.

Sweller identified three types of cognitive load:

- **Intrinsic load:** Determined by the inherent complexity of the material and the learner's prior knowledge. High element interactivity (many concepts that must be understood simultaneously) drives high intrinsic load. Cannot be eliminated — only managed by sequencing simpler elements before complex ones.
- **Extraneous load:** Imposed by poor instructional design — unnecessary information, redundant representations, or confusing formats. Does not contribute to learning; competes directly with intrinsic load for working memory capacity. The primary target for instructional design optimization.
- **Germane load:** Cognitive effort directed toward schema construction — building organized knowledge structures in long-term memory that can later be retrieved as single chunks, effectively expanding working memory's functional capacity. Good instruction maximizes germane load within the remaining capacity.

## Key Properties

- **Working memory bottleneck:** The constraint is not the material itself but the simultaneous activation of interacting elements in working memory
- **Schema automation:** Repeated successful retrieval of a schema reduces its working memory cost — it becomes a single chunk rather than many elements
- **Expertise reversal effect:** Worked examples (low extraneous load) benefit novices but impair experts, whose schemas make the guidance redundant and add extraneous load
- **Split-attention effect:** Spatially or temporally separated sources of mutually referring information impose extraneous load by requiring mental integration; physically integrating them reduces this cost
- **Redundancy effect:** Providing the same information in two formats simultaneously can increase extraneous load for learners who can process either format alone

## Connections

VaultMind's [[context-pack|Context Pack]] is a cognitive load manager. The token budget directly implements the working memory capacity constraint — it caps how much information enters the agent's context window (the LLM's working memory). Priority ordering and semantic filtering ensure that the most relevant content fills the available capacity first, minimizing extraneous load from tangential notes. By doing the assembly work outside the agent, VaultMind shifts burden from the agent's inference pass to a preprocessing stage — analogous to how worked examples reduce in-task cognitive load by offloading problem-solving structure to the instructional material.
