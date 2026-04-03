---
id: concept-entity-resolution
type: concept
title: Entity Resolution
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Record Linkage
  - Entity Matching
  - Deduplication
tags:
  - knowledge-representation
  - graph-theory
  - vaultmind
related_ids:
  - concept-semantic-networks
  - concept-associative-memory
source_ids: []
---

## Overview

Entity resolution is the process of determining whether different references point to the same real-world entity. In databases, this is "record linkage" — matching records across tables without a shared key. In knowledge graphs, it's mapping surface-form references (names, aliases, descriptions) to canonical node identifiers.

## Key Properties

- **Surface-form variation:** The same entity can be referenced by many names ("United States", "US", "USA", "America")
- **Ambiguity:** The same name can refer to different entities ("Mercury" = planet, element, or god)
- **Cascading resolution:** Resolving one entity can help resolve others (knowing "Dr. Anderson" is "John Anderson" disambiguates "Anderson's model")
- **Confidence levels:** Resolution can be certain, probable, or ambiguous

## Connections

VaultMind implements a five-tier entity resolution cascade: exact ID match > exact title > exact alias > normalized (case-insensitive) > unresolved. When multiple notes match at the same tier, VaultMind returns all candidates with `ambiguous: true` rather than silently picking one.

This is a deliberate design choice — in a knowledge management system, silent disambiguation is worse than surfacing ambiguity. The agent or human can then choose the correct entity with full information.
