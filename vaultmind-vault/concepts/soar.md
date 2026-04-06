---
id: concept-soar
type: concept
title: SOAR
created: 2026-04-07
vm_updated: 2026-04-07
aliases:
  - State, Operator, and Result
  - SOAR Architecture
  - Newell's SOAR
tags:
  - cognitive-science
  - cognitive-architecture
  - memory-models
related_ids:
  - concept-act-r
  - concept-working-memory
  - concept-associative-memory
source_ids:
  - source-newell-1990
---

## Overview

SOAR (State, Operator, and Result) is a cognitive architecture developed by Allen Newell and colleagues, presented comprehensively in Newell's 1990 book *Unified Theories of Cognition*. It was designed as a general theory of cognition — a single computational framework capable of accounting for the full range of intelligent behavior, from simple reaction time tasks to multi-step problem solving and language understanding.

SOAR's central metaphor is problem-space search. All cognition is represented as the application of operators to states within problem spaces. The working memory holds the current state; long-term procedural memory (implemented as a production system) supplies operators; and the system selects and applies operators to move toward goal states. When the system cannot select an operator — an *impasse* — it automatically creates a subgoal and opens a new problem space to resolve it. This impasse-driven subgoaling is SOAR's universal mechanism for handling novelty and complexity.

The learning mechanism in SOAR is **chunking**: when an impasse is resolved, SOAR automatically summarizes the processing that resolved it into a new production rule (chunk) stored in procedural long-term memory. This means SOAR learns by doing — every resolved impasse creates a new chunk that prevents the same impasse from occurring in similar future situations. Alongside [[act-r|ACT-R]], SOAR is one of the two architectures that have dominated cognitive architecture research, though they differ markedly in approach: ACT-R is more modular and mathematically precise; SOAR is more unified and architecturally minimalist.

## Key Properties

- **Problem-space formalism:** All cognition is operator application within problem spaces; goals are states to be achieved, operators are the production rules that transform states
- **Impasse-and-subgoal mechanism:** When operator selection fails (tie, no-change, or conflict), SOAR automatically creates a subgoal in a new problem space to resolve it — this is the architecture's sole control mechanism
- **Chunking as learning:** Resolved impasses automatically generate new production rules; SOAR learns procedurally by experiencing and resolving novel situations
- **Long-term memory partitions:** SOAR distinguishes procedural memory (production rules), semantic memory (facts, accessible via declarative retrieval), and episodic memory (records of prior states, retrievable for analogical reasoning)
- **Working memory as the cognitive bus:** All active information — current goals, retrieved facts, perceptual inputs — flows through working memory; long-term memory is inactive until retrieved into working memory

## Connections

SOAR's three-way partition of long-term memory (procedural, semantic, episodic) maps onto VaultMind's note type hierarchy. Concept notes are semantic memory — facts about the world, organized by meaning. Decision notes are procedural-like — they encode rules, constraints, and rationale that guide future agent behavior. The episodic trace of how the vault was built lives in the creation timestamps and `source_ids` linkages.

SOAR's working memory maps to the agent's context window (see [[working-memory|Working Memory]]), and the [[context-pack|Context Pack]] mechanism is analogous to SOAR's declarative memory retrieval: a deliberate, cue-driven query that loads a specific memory into working memory so it can be used in the current problem-solving episode. SOAR's chunking mechanism suggests a future VaultMind pattern: agents that resolve a novel problem should be able to chunk the solution into a new decision note automatically.
