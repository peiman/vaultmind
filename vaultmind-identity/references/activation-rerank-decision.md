---
id: reference-activation-rerank-decision
type: reference
title: "Activation as Rerank — Slice 5b'' Design Decision"
created: 2026-05-03
tags:
  - activation
  - retrieval
  - rrf
  - plasticity
  - slice-5b
related_ids:
  - reference-plasticity-priority-order
  - reference-current-context
  - reference-baseline-2026-04-28
  - reference-probe-before-commit
  - principle-measure-before-optimize
  - arc-the-lighter-move-is-the-work
  - arc-closing-the-ranking-bug-at-the-right-layer
---

# Activation as Rerank — Slice 5b'' Design Decision

The original slice 5b' (commit `499cbef`, 2026-05-01) appended `ActivationRetriever` as a 5th lane in `HybridRetriever`'s RRF combine. The probe at commit `e29ee10` showed retrieval **degraded**. This document captures the 2026-05-03 probe sequence that diagnosed the cause, the candidate fixes considered, and the decision to ship slice 5b'' as a **post-RRF rerank** instead. Implementation follows in a separate commit.

## What 5b' did and why it failed

Slice 5b' built `ActivationRetriever` (`internal/query/activation_retriever.go`) — a query-independent retriever that ranks notes by ACT-R activation score (base-level activation + decay). It was wired as the 5th lane alongside dense / sparse / colbert / fts in `BuildAutoRetrieverWithActivation`. RRF combine in `HybridRetriever` uses **mean-of-present**: for each note, sum `1/(K+rank+1)` across lanes it appears in, then divide by lane-count-present.

The activation lane returns ALL accessed notes (every note with `access_count > 0`), regardless of query. This is by design — activation is query-independent.

The docstring on `ActivationRetriever` claimed (lines 17-25): "The mean-of-present RRF fusion in HybridRetriever then only boosts notes that ALSO appear in at least one query-dependent lane." **This claim is wrong.** Mean-of-present divides by present-lane-count; it does not intersect across lanes. A note appearing in only the activation lane at rank 1 still gets a single-lane RRF score, which can match or beat a multi-lane note's score. The docstring described an intersection behavior that the math does not implement.

## Probe sequence (2026-05-03)

### P1 — Re-baseline

Re-ran `VAULTMIND_ACTIVATION_COMPARE=identity|research` to confirm the e29ee10 numbers were still representative.

| Vault | 4-way Hit@5 | 5-way Hit@5 | 4-way MRR | 5-way MRR | Δ MRR |
|---|---|---|---|---|---|
| identity (n=19) | 0.895 | 0.895 | 0.816 | 0.692 | **-0.124** |
| research (n=40) | 0.975 | 0.900 | 0.822 | 0.555 | **-0.268** |

Slightly less degraded than e29ee10 baseline (more access events accumulated), but the shape is the same. Identity has 15 ▼ queries vs 2 ▲. Research is dominated by ▼.

### P2 — Mechanism (top-5 dump for worst queries)

Instrumented the compare test temporarily to dump top-5 result IDs for queries with `|Δrr| ≥ 0.4`. The pattern:

**Identity vault — "who is Peiman" Δrr -0.800.**
- 4-way top-5: `identity-peiman` (✓), arc-workhorse-collaboration, arc-the-breakthrough, arc-plasticity-gap-from-inside, arc-thinking-with-peiman.
- 5-way top-5: arc-workhorse-collaboration, arc-the-breakthrough, **identity-who-i-am**, arc-thinking-with-peiman, `identity-peiman` (✓ at rank 5).

`identity-who-i-am` has access_count=10 (vault's most-touched note). It was NOT in the 4-way top-5 for "who is Peiman". The activation lane lifted it INTO top-5 of the 5-way result because the activation rank-1 score plus any cross-lane appearance at any rank cleared the mean-of-present threshold.

**Research vault — "what should I review and when to maximize retention" Δrr -1.000 (out of top-5 entirely).**
- 4-way top-5: `concept-spaced-repetition-algorithms` (✓), concept-spacing-effect, concept-sleep-spacing-interaction, concept-retrieval-evaluation-metrics, source-mazza-2016.
- 5-way top-5: **concept-spreading-activation, concept-hebbian-learning**, concept-spacing-effect, concept-sleep-spacing-interaction, **concept-act-r**.

`concept-spreading-activation` (count=11), `concept-hebbian-learning` (count=7), `concept-act-r` (count=1) all appear in top-5 of a query about a completely unrelated topic. The expected note dropped out entirely.

**Access count distribution explains the asymmetry.**

- Identity vault: 18 of 32 notes accessed (56% coverage).
- **Research vault: 10 of 407 notes accessed (2.5% coverage).** The activation lane is essentially ranking 10 random notes against queries that should hit any of 407. For any unrelated query, those 10 notes saturate the top-5 of the 5-way result.

### P2 — The math made concrete

Worked example: a note in **activation-only at rank 1** (1 lane present) scores `(1/61) / 1 = 0.0164`. A note in **4 query lanes at rank 1 each** (4 lanes present) scores `4 * (1/61) / 4 = 0.0164`. **They tie.**

This is the structural flaw: mean-of-present treats one-high-rank-lane and four-high-rank-lanes as equivalently strong. With activation as a parallel lane, any activation-hot note that ALSO appears in any query lane at any rank wins via tie-break against the genuinely-relevant note.

### P3 — Mean-of-K probe (one-line fusion change)

Replaced `score /= len(components)` with `score /= len(retrievers)` (always divide by total lanes, present or not). Re-ran both vaults.

| Vault | 4-way MRR | 5-way MRR | Δ MRR | Compared to mean-of-present |
|---|---|---|---|---|
| identity | 0.769 | 0.733 | -0.036 | Δ shrunk from -0.124, **but 4-way also dropped 0.047** |
| research | 0.822 | 0.741 | -0.081 | Δ shrunk from -0.268 (~70% rescued); 4-way unchanged |

Mean-of-K rescues most of the research regression — 70% of MRR loss recovered — at the cost of degrading the 4-way identity baseline. It's a different fusion philosophy that punishes notes appearing in fewer lanes (penalizes any note not in all lanes). Affects all queries, not just activation-affected ones. **Not a clean win.**

## Candidate fixes — why each was rejected or accepted

### Option A — Constrain ActivationRetriever to 4-way candidates

Original priority-order doc intent: activation lane scores only notes that appear in the 4-way result. Implementation drift was the lane returning ALL accessed notes.

**Rejected as architecture, accepted as semantics.** The semantic intent (activation operates on 4-way candidates only) is correct. But forcing this through the parallel-lane API means `ActivationRetriever.Search` has to know other retrievers' results — breaks the abstraction. Functionally same as B; B's framing is cleaner.

### Option B — Post-RRF rerank ✅

Run the 4-way RRF as today. Take the top-N candidates. Apply an activation-aware second pass that blends RRF score with activation score using tunable weights `α` (RRF) and `β` (activation). Reorder the N candidates; return top-K.

**Accepted.** Reasons:
- 4-way fusion math is preserved (no degradation of the existing baseline).
- Activation operates only on candidates that survived 4-way RRF — drown-out is structurally impossible.
- Activation's role is honestly second-order: it reranks within the candidate set, doesn't introduce new candidates.
- Tunable weights — empirical α / β / N selection.
- Matches the priority-order doc's intent without the abstraction violation.

### Option C1 — Require ≥2 lanes to score

Filter mean-of-present to only count notes appearing in at least 2 lanes. Drops single-lane contributions.

**Rejected.** P2 data shows the drown-out mechanism is activation-hot notes appearing in 2+ lanes (activation + at least one query lane). The ≥2-lane filter doesn't catch them. Also hurts legitimate single-lane wins (rare-term FTS exact matches).

### Option C3 — Mean-of-K (always divide by total lanes)

Tested via probe P3 above.

**Rejected.** Partial rescue at the cost of degrading the 4-way baseline. Mean-of-K is a different fusion philosophy that affects all queries; the cost-benefit isn't clean. Useful as a probe data point — confirms that fusion-math fixes alone don't reach the structural problem (the candidate set).

## Slice 5b'' — what gets implemented

`internal/query/activation_reranker.go` — a wrapper retriever:

```
ActivationReranker wraps a base retriever (the 4-way HybridRetriever).
Search:
  1. Call base.Search(query, N).  // N = max(K, 10) where K is requested limit
  2. For each candidate c in 1..N:
     rrf_norm[c]        = c.score / max_rrf_score
     activation_raw[c]  = ComputeApproxRetrieval(c.count, c.last_accessed, now, params)
                          (0 if c.count == 0)
     activation_norm[c] = activation_raw[c] / max_activation_score
                          (0 if max_activation == 0)
     final[c]           = α * rrf_norm[c] + β * activation_norm[c]
  3. Sort candidates by final desc.
  4. Return top-K.
```

**Corner cases handled:**
- No accessed notes in candidates → `activation_norm` all 0 → `final = α * rrf_norm` → identical to 4-way. Safe.
- All accessed → activation contributes meaningfully within the candidate set.
- 4-way ties → activation breaks ties.
- Right answer at rank N+1 → not rescuable; mitigation is choosing N ≥ 2*K.
- Activation is query-independent in source but query-dependent through the candidate filter — activation only matters for notes that already cleared 4-way.

## What stays on disk

The existing `ActivationRetriever` + `BuildAutoRetrieverWithActivation` (5th-lane variant) **stays on disk, unwired, opt-in via existing config**. Same preservation pattern as `vault-block-read.sh` per `arc-the-lighter-move-is-the-work` — the approach that didn't pan out remains as evidence of why the new approach exists. Future instances can read the comparison probe's history and the docstring (which will be updated to clearly mark the lane variant as superseded) to inherit the reasoning.

## α / β / N probe plan

Three weight pairs to test, fixed N=10:

| Variant | α (RRF) | β (activation) | Hypothesis |
|---|---|---|---|
| 0.5 / 0.5 | 0.5 | 0.5 | Activation gets equal weight — likely too aggressive given the structural drown-out tendency. |
| 0.7 / 0.3 | 0.7 | 0.3 | RRF dominant, activation as a tie-breaker / soft-lift. Starting hypothesis. |
| 0.9 / 0.1 | 0.9 | 0.1 | RRF strongly dominant, activation barely shifts ranks — closest to no-rerank baseline. |

**Success criteria**: pick the variant that maximizes MRR on identity AND research, while NOT degrading 4-way baseline. The compare test gives us the side-by-side. Run all three; record numbers; pick the dominator.

If two variants tie within noise, prefer the higher-α (closer to no-rerank). Manifesto principle 4: reality is the spec. Reality might say "activation contributes very little; α=0.9 is fine." Or it might say "activation is the difference between rank 2 and rank 1; α=0.5 wins on the ▲ cases."

**Probe gate**: if no variant beats 4-way alone on either vault, the rerank shape itself is wrong — go back to design. If at least one variant beats 4-way on at least one vault without degrading the other, ship that variant as the default.

## Probe results (2026-05-03)

First pass with score-normalized blending **failed catastrophically on identity** (Hit@5 dropped from 0.895 → 0.368 at α=0.5/β=0.5). Deep-dive showed broad anchor notes (`reference-current-context` count=5, `identity-who-i-am` count=10) appear in 4-way top-10 for many queries. Once there, score-normalization stretched their activation contribution to 1.0 (max within candidate set) while RRF normalized contribution was only ~0.5-0.8 for top-1 hits. Activation crushed legitimate top-1s.

**Pivoted to rank-based RRF blending** — both lanes scored on the same `1/(K+rank+1)` scale (K=60). Bounds each lane's contribution; eliminates the normalization-amplification bug. Re-ran the sweep:

| Variant | identity ΔHit@5 | identity ΔMRR | research ΔHit@5 | research ΔMRR |
|---|---|---|---|---|
| α=0.5/β=0.5 | -0.263 | -0.304 | 0.000 | -0.067 |
| α=0.7/β=0.3 | 0.000 | -0.193 | 0.000 | -0.037 |
| **α=0.9/β=0.1** | **0.000** | **-0.053** | **0.000** | **0.000** ← winner |
| α=0.95/β=0.05 | 0.000 | -0.027 | 0.000 | 0.000 (β no-op) |

**α=0.9/β=0.1 wins.** Research vault stays at parity with 4-way (Hit@5 0.975, MRR 0.822). Identity vault loses 0.053 MRR but Hit@5 holds. α=0.95/β=0.05 reduces identity loss further but makes β effectively a no-op — research gets no benefit either.

**Comparison with the abandoned 5b' lane variant**:

| Vault | 5b' lane (mean-of-present) | 5b'' rerank α=0.9/β=0.1 |
|---|---|---|
| identity ΔMRR | -0.124 | **-0.053** (57% better) |
| research ΔHit@5 | -0.075 | **0.000** (drown-out eliminated) |
| research ΔMRR | -0.268 | **0.000** (full parity) |

The structural fix worked. Activation no longer introduces drown-out candidates. The residual identity-MRR loss is from broad anchor notes legitimately in the 4-way top-10 getting promoted by activation; this is a vault-shape consequence, not a structural bug.

## Honest verdict on activation-in-retrieval

The probe data tells an uncomfortable truth: **activation-in-retrieval is structurally hard when access distribution is dominated by broad anchor notes.** No β value simultaneously helps research and doesn't hurt identity in this vault. The trade-off is real and not eliminable by fusion-math choices alone.

The 5b'' shape (rerank, rank-based blend, α=0.9/β=0.1) is the **best probed point** on this trade-off. It's a clear improvement over the 5b' lane variant. But it's not free — identity-vault MRR still loses 0.053 vs pure 4-way.

**Shipping decision**: same opt-in gate as 5b'. The rerank infrastructure ships; the default-on switch waits for the calibrated-confidence threshold re-probe (step 4 first slice was calibrated against 4-way distribution; rank-based blending shifts the rank-1/rank-2 score gap distribution). Same step-4 ↔ step-5 coupling that gated 5b'. The reranker is callable via `query.BuildAutoRetrieverWithRerank` but is not wired as the default `BuildAutoRetriever` path.

When access distribution evolves (Siavoush dogfooding adds breadth; the broad-anchor dominance flattens), re-run the sweep. Reality may say β=0.1 is fine on a more balanced vault, or that β should adapt to access-distribution shape.

**Future probe candidates** (deferred until evidence demands them):
- Per-vault adaptive β based on access-count distribution shape (Gini coefficient, top-N concentration).
- Activation source variation: count-only vs last-accessed-only vs combined ACT-R formula. Probe might show last-accessed-only is less broad-anchor-biased than count.
- Per-query activation: what's "warm" for THIS topic, not "warm" globally. Requires topic-conditional activation modeling — open research direction.

## Confidence calibration re-probe (2026-05-03)

Per the shipping decision above ("default-on switch waits for the calibrated-confidence threshold re-probe"), ran the same gold-query corpora through both 4-way and 5b''-rerank with α=0.9/β=0.1 to capture the rank-1/rank-2 score gap distribution under each. Histograms classified by current thresholds (5% strong / 1.5% moderate / 0.5% weak / below = no_match):

| Vault | 4-way: strong / moderate / weak / no_match | 5b'' rerank: strong / moderate / weak / no_match |
|---|---|---|
| identity (n=19) | 1 / 16 / 2 / 0 | **0 / 5 / 8 / 6** |
| research (n=44) | 2 / 25 / 10 / 3 | **0 / 4 / 34 / 2** |

**Distribution massively compressed under rerank.** Canonical-strong queries collapse:
- "Hebbian learning" — 4-way 5.66% strong → rerank 2.98% moderate.
- "memory that retrieves by content not by address" — 4-way 7.32% strong → rerank 1.47% weak.
- "session catches mirror material" — 4-way 5.08% strong → rerank 0.25% no_match.

**Mechanism**: rank-based RRF blending (α=0.9/β=0.1) confines each lane's contribution to the bounded `1/(K+rank+1)` range. Top-N candidate scores cluster within a narrow window; raw gap percentages compress 3-10x relative to 4-way. The threshold values calibrated against the 4-way distribution don't apply.

**Decision: gate is real and confirmed.** Slice 5b'' MUST stay opt-in until thresholds are re-calibrated. Wiring `BuildAutoRetrieverWithRerank` as the default `BuildAutoRetriever` path right now would silently mis-label almost every result as weak/no_match.

**What proper re-calibration requires** (deferred — separate work):
1. A new probe query set built for the blended distribution: canonical-match queries (where the right answer is unambiguously rank-1 with strong gap), failure-mode queries (rank-2/rank-6 cases), nonsense queries (no real winner). The 4-way set may not be reusable verbatim — gaps measure something different now.
2. Actually run those queries through `BuildAutoRetrieverWithRerank` and measure where the gap-population separation lives.
3. Pin new thresholds via a 5b''-specific calibration test (mirror of `TestComputeTopHitConfidence_ProbedQueries_2026_04_30`).
4. Add retriever-aware threshold selection in `computeTopHitConfidence` — different retrievers, different thresholds.

**Probe-tooling artifact added**: `TestConfidenceCalibrationProbe` (env-gated `VAULTMIND_CONFIDENCE_CALIBRATION=identity|research`) in `internal/baseline/activation_compare_test.go`. Reusable for the proper re-calibration work; current run produced the histograms above.

**Bottom line for shipping**: 5b'' stays callable via `BuildAutoRetrieverWithRerank`, NOT default. The calibration gate held. Future re-calibration work is anchored — the probe tooling exists and the data shows a shift large enough to matter.

## What this is not

- **Not a final calibrated-confidence fix** (step 4 of the plasticity roadmap). The TopHitConfidence thresholds were calibrated against 4-way distribution. Slice 5b'' shifts the rank-1/rank-2 score gap distribution for queries where activation contributes; thresholds need re-probing post-5b''. Same step-4 ↔ step-5 coupling caught while dogfooding 5b'.
- **Not a federation-ready design.** Cross-vault federation (step 5.5, see `reference-federation-architecture`) reranks AT a higher layer. 5b'' operates within a single vault. Composition is forward-compatible: federation merges per-vault top-K (each vault already reranked); cross-vault rerank is an additional layer.
- **Not a defense-in-depth on top of the 5th-lane code.** The 5th-lane code stays parked, unwired. 5b'' replaces it as the default activation path; the lane variant is the documented escalation we don't take.

## Source

- Conversation date: 2026-05-03.
- Originating prompt: "do some probing on how we should do this. have you done enough research?" — the message that made me realize I was about to recommend B from a position of inadequate evidence.
- Probe-before-commit principle: `reference-probe-before-commit`. Three probes shifted the question from "what do I think" to "what does reality say."
- Companion arc: `arc-the-lighter-move-is-the-work` — the discipline this work honors. The lighter move is to fix slice 5b' at the layer where the structural flaw lives (candidate set, not fusion math), not to ship more aggressive fusion changes.
- Predecessor: `reference-current-context` (named slice 5b' as the live edge needing redesign; option B was the recommendation in flight).
- Implementation artifacts (forthcoming): `internal/query/activation_reranker.go` and the α/β probe results.
