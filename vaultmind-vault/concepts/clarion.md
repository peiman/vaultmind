---
id: concept-clarion
type: concept
title: CLARION
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - CLARION Architecture
  - Connectionist Learning with Adaptive Rule Induction
tags:
  - cognitive-science
  - cognitive-architecture
related_ids:
  - concept-act-r
  - concept-soar
  - concept-working-memory
source_ids: []
---

## Overview

CLARION (Connectionist Learning with Adaptive Rule Induction On-line) is a cognitive architecture developed by Ron Sun, first presented in the mid-1990s and comprehensively described in "Anatomy of the Mind" (Oxford University Press, 2016). Its defining feature is a principled two-level representational distinction between explicit and implicit knowledge, both of which are modeled as computational subsystems operating in parallel.

The explicit level encodes rule-based, declarative, verbalizable knowledge — the kind that can be introspected and articulated. The implicit level encodes subsymbolic, proceduralized knowledge in connectionist networks — the kind that shapes behavior without being accessible to conscious inspection. This explicit/implicit distinction maps onto the broader dual-process tradition in cognitive science (System 1 / System 2) but is more mechanistically specified.

## Key Properties

CLARION comprises four interacting subsystems:

- **Action-Centered Subsystem (ACS):** Controls action selection; contains both explicit action rules (top level) and implicit procedural knowledge (bottom level). This is the primary locus of skill learning.
- **Non-Action-Centered Subsystem (NACS):** Stores declarative knowledge — facts, beliefs, and episodic traces — again at both explicit and implicit levels.
- **Motivational Subsystem (MS):** Represents drives, goals, and affect that modulate the behavior of the other subsystems.
- **Meta-Cognitive Subsystem (MCS):** Monitors and regulates the other subsystems; adjusts learning rates, triggers strategy shifts, and oversees goal management.

The key innovation is **bottom-up learning**: patterns that recur at the implicit level can be automatically extracted and articulated as explicit rules, allowing procedural skill to become declarative knowledge over time. This is the inverse of the standard skill-acquisition story (in which explicit rules are practiced until they become automatic), and it models insight and metacognitive awareness.

## Connections

CLARION's explicit/implicit distinction maps directly onto a design dimension in VaultMind. Explicit links in the vault are human-authored wikilinks — verbalizable, inspectable, typed. Implicit edges are inferred by the engine: alias mentions, tag co-occurrence, and embedding proximity. These are subsymbolic — present in the data but not articulated as named relations.

The bottom-up learning idea in CLARION suggests a concrete VaultMind v2 direction: automatic promotion of implicit edges to explicit relation entries when the inferred connection is strong and stable enough. A pattern of tag overlap and mutual citation between two notes could be surfaced to the vault owner as a suggested explicit link — CLARION's extraction-to-explicit-rule mechanism applied to knowledge graph maintenance. Compare [[act-r|ACT-R]] (activation-based retrieval) and [[soar|SOAR]] (problem-space search) for the broader cognitive architecture landscape.
