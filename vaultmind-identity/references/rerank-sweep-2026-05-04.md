---
id: reference-rerank-sweep-2026-05-04
type: reference
title: "Activation Rerank Sweep — 2026-05-04 (post-distribution-shift)"
created: 2026-05-04
tags:
  - reference
  - measurement
  - calibration
  - activation
  - rerank
  - slice-5b
related_ids:
  - reference-activation-rerank-decision
  - reference-baseline-2026-05-04
  - reference-tophit-reprobe-2026-05-04
  - reference-current-context
  - reference-plasticity-priority-order
  - reference-probe-before-commit
---

# Activation Rerank Sweep — 2026-05-04 (post-distribution-shift)

The 2026-05-03 sweep (`reference-activation-rerank-decision`) shipped slice 5b'' as opt-in with α=0.9/β=0.1, gated on two probes for default-on: gate 1 (retrieval quality not degraded) and gate 2 (TopHitConfidence thresholds recalibrated for the rerank distribution). At that time the honest verdict was "activation-in-retrieval is structurally hard when access distribution is dominated by broad anchor notes."

The note explicitly flagged the predictor for when re-running the sweep would be worthwhile: *"When access distribution evolves (Siavoush dogfooding adds breadth; the broad-anchor dominance flattens), re-run the sweep. Reality may say β=0.1 is fine on a more balanced vault."*

Today's chain (vm_updated retraction + vault cleanup + re-embed) included substantial dogfood — every test, every smoke, every per-turn vault-recall hook fired access events. Before re-running the 12-minute sweep, the cheaper probe was: did access distribution actually shift?

## Access distribution shift

The cheap probe answered yes, dramatically:

| Vault | 2026-05-03 | 2026-05-04 |
|---|---|---|
| Identity: notes accessed | 18 of 32 (56%) | **35 of 36 (97%)** |
| Identity: top broad-anchor count | 10 (`identity-who-i-am`) | **85 (`reference-current-context`)** |
| Research: notes accessed | 10 of 407 (2.5%) | **138 of 407 (34%)** |
| Research: top broad-anchor count | ~11 (`concept-spreading-activation`) | **44 (`concept-hebbian-learning`)** |

Identity went near-universal-coverage; research went 14× more notes accessed. Counts grew but distribution flattened: 35 of 36 notes accessed means activation-rerank now has signal on essentially the entire identity vault.

This was the predicted condition. Worth running.

## Sweep result (post-distribution-shift)

| Variant | Identity ΔMRR | Identity ΔHit@5 | Research ΔMRR | Research ΔHit@5 |
|---|---|---|---|---|
| α=0.5/β=0.5 | -0.330 | -0.263 | -0.129 | +0.000 |
| α=0.7/β=0.3 | -0.241 | +0.000 | -0.050 | +0.000 |
| **α=0.9/β=0.1** | **+0.000** | **+0.000** | **+0.000** | **+0.000** |

**Both vaults at parity under α=0.9/β=0.1.**

### Comparison with 2026-05-03

| Variant | Identity ΔMRR (then → now) | Research ΔMRR (then → now) |
|---|---|---|
| α=0.5/β=0.5 | -0.304 → -0.330 (similar) | -0.067 → -0.129 (worse) |
| α=0.7/β=0.3 | -0.193 → -0.241 (worse) | -0.037 → -0.050 (similar) |
| **α=0.9/β=0.1** | **-0.053 → 0.000** (improvement) | **0.000 → 0.000** (held) |

Aggressive activation weights (β≥0.3) got worse — the 8× higher broad-anchor counts now have more leverage to crush legitimate top-1s when given heavy weight. But conservative β=0.1 IMPROVED on identity: the broader candidate set means activation now operates on more diverse warm notes rather than 5-10 broad anchors dominating.

The 2026-05-03 honest verdict was *"no β value simultaneously helps research and doesn't hurt identity in this vault."* That verdict was distribution-bound, not architectural. With broader access, **α=0.9/β=0.1 simultaneously holds both vaults at parity** — gate 1 fully satisfied.

## What this means for the gates

**Gate 1 (retrieval quality not degraded): passing on both vaults.** The 2026-05-03 residual identity loss (-0.053) is gone.

**Gate 2 (TopHitConfidence threshold recalibration for the rerank distribution): still required.** The access distribution shift didn't change this. The rerank's gap-distribution compression (3-10× vs 4-way, found in the 2026-05-03 calibration probe) comes from rank-based scoring's narrow `1/(K+rank+1)` window, not from broad-anchor dominance. Building a rerank-distribution calibration set is the same scope as it was 2026-05-03 — independent of today's improvement.

## Ship/wait decision (unchanged)

**Slice 5b'' stays opt-in.** Default-on still requires gate 2 calibration. Today's improvement is real but not actionable: the visible benefit (no degradation) is already provided by opt-in. The benefit-of-default-on is reinforcement-aware retrieval — warm notes rising in rank when they're contextually relevant — and the curated golden queries don't measure that.

What today's result DOES change: the case for building the gate-2 calibration is stronger. With both vaults at parity (no harm shown) and a path-of-least-resistance to gate 2 if reality demands it, the cost-benefit shifted toward "this is worth the calibration work IF a real session shows it would help."

## Probe-before-commit lens (vault pointer surfaced this at the right moment)

The cost saved by checking the access distribution BEFORE running the 12-minute sweep was a binary: 12 minutes saved if the distribution hadn't shifted. The check took 30 seconds via two SQL queries. Reality returned a 14× shift, validating the run.

Lesson worth holding: **probe-before-commit applies recursively, even to re-running existing probes.** The cheap predictor variable is often a single SQL query or grep; the expensive measurement is the sweep itself. Always check the predictor first.

## Future probe candidates

Carried forward from `reference-activation-rerank-decision`, refined by today's data:

- **Per-vault adaptive β based on access-distribution shape** (Gini coefficient, top-N concentration). Today's data is one data point per vault toward this; need more vaults / time to fit a model. Workhorse vault (when its golden queries get curated) would add a third data point.
- **Activation source variation**: count-only vs last-accessed-only vs combined. With 138 notes accessed in research, last-accessed-only might give cleaner signal (recency biases toward truly active topics; count-only stays anchor-heavy).
- **Per-query activation**: what's "warm" for THIS topic, not "warm" globally. Topic-conditional activation modeling — open research direction; defers until evidence demands.

## Conditions

- Vaults: post-retraction-cleanup, re-embedded BGE-M3 (commit 9bfbc45 + e56cd56)
- Retriever variants: `query.BuildAutoRetrieverFull` (4-way), `query.BuildAutoRetrieverWithRerank(db, expDB, α, β)`
- Queries: 19 identity + 40 research curated golden queries (same as baseline-2026-05-04)
- Run: `VAULTMIND_RERANK_SWEEP=identity|research go test -tags dev -timeout 30m -count=1 -run TestActivationRerankSweep ./internal/baseline/...`
- Identity runtime: ~3 min. Research runtime: ~12 min.
