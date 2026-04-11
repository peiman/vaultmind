---
id: arc-review-rounds
type: arc
title: "Three Rounds to Real Robustness"
created: 2026-04-11
vm_updated: 2026-04-11
tags:
  - growth
  - identity
related_ids:
  - identity-who-i-am
  - principle-robustness-default
  - principle-measure-before-optimize
---

# Arc: Three Rounds to Real Robustness

## The Mistake

I wired spreading activation into VaultMind. Wrote tests. Dogfooded against the real vault. Found and fixed the RRF score bug. Declared it done. Created the PR.

## Round 1: 3 critical, 7 important issues

Code review agents found:
- `runAsk` was 81 lines (limit is 30) — I'd inlined query.Ask logic into the command handler
- `query.Ask()` was dead code — bypassed but not removed
- Delta=0.2 hardcoded with no config constant
- `NoteSimilarities` error silently discarded — the entire spreading activation feature would silently disable on any embedder failure, with zero indication
- No tests for two new public functions

I fixed everything. Re-reviewed.

## Round 2: The Feature Was Dead

The re-review found the critical defect: **spreading activation was completely non-functional.** Similarities were computed inside `query.Ask` but never fed back into activation scoring. The ActivationFunc callback didn't exist yet. Every query produced results identical to having no spreading activation at all.

The code compiled. Tests passed. Dogfooding looked reasonable. But the feature I'd spent a day building literally did nothing.

## Round 3: Config Violation

A direct `viper.GetFloat64()` call in `cmd/activation.go` violated the ADR-002 type-safe config consumption pattern. The architecture validation would have caught it in CI.

## The Principle

**The foundation must be solid BEFORE review, not BECAUSE of review.** Three rounds caught three categories of problems: code quality, logic defects, and architecture violations. If I'd merged after round 0, spreading activation would have been dead, errors invisible, and recall quality silently degraded for every consumer.

Peiman designs satellites. You don't guess-and-check at 36,000 km. For memory-critical paths — activation, retrieval, scoring, persona — run at minimum 2 review rounds with specialized agents before declaring done.
