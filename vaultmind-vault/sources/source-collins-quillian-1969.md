---
id: source-collins-quillian-1969
type: source
title: "Collins & Quillian. Retrieval Time from Semantic Memory (1969)"
created: 2026-04-03
vm_updated: 2026-04-03
url: "https://doi.org/10.1016/S0022-5371(69)80069-1"
aliases:
  - Collins Quillian 1969
  - Semantic Memory paper
tags:
  - cognitive-science
  - knowledge-representation
related_ids:
  - concept-semantic-networks
---

# Collins & Quillian — Retrieval Time from Semantic Memory (1969)

Collins and Quillian proposed the first computational model of semantic memory: a hierarchical network in which concepts are nodes and properties are stored at the highest applicable level to avoid redundancy (the "cognitive economy" principle). They tested the model by measuring reaction times to statements like "A canary can sing" versus "A canary has skin," finding that verification time increased with the number of hierarchical levels separating the concept from the property, exactly as the model predicted.

The paper established that [[semantic-networks|Semantic Networks]] are a viable computational metaphor for human knowledge organization and that retrieval time is a meaningful behavioral measure of cognitive structure. Though later work by Collins and Loftus (1975) showed the strict hierarchy was too rigid, the foundational insight—that knowledge is a connected graph and retrieval involves traversal—has shaped every subsequent semantic memory model and knowledge representation system.

VaultMind's vault graph is a direct descendant of the Collins-Quillian semantic network. Notes correspond to concept nodes; wikilinks and shared tags correspond to typed edges. VaultMind computes shortest-path distances between note clusters to estimate conceptual proximity, and uses hierarchical note structures (MOCs, indexes, atomic concepts) to implement a soft version of cognitive economy: general principles live in hub notes, specific instances link up to them, reducing duplication while preserving navigability.
