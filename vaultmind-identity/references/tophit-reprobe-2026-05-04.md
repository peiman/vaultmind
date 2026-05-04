---
id: reference-tophit-reprobe-2026-05-04
type: reference
title: "TopHitConfidence Re-probe — 2026-05-04 (post-retraction)"
created: 2026-05-04
tags:
  - reference
  - measurement
  - calibration
  - confidence
related_ids:
  - reference-baseline-2026-05-04
  - reference-current-context
  - reference-plasticity-priority-order
---

# TopHitConfidence Re-probe — 2026-05-04 (post-retraction)

The current TopHitConfidence thresholds (5% strong / 1.5% moderate / 0.5% weak / <0.5% no_match) were calibrated on 2026-04-30 against the 4-way RRF distribution. Today's retraction chain regenerated every embedding (vm_updated stripped from 447 notes → content hashes changed → re-embed bge-m3-uniform). The implicit hypothesis was: "the gap distribution that calibrated those thresholds didn't shift underneath." Re-probing the canonical 2026-04-30 query set against the post-retraction state to falsify or confirm.

## Result

**Thresholds held. No code change needed.** 8 of 9 probe queries reproduced their 2026-04-30 gap to the second decimal; the ninth shifted within the same tier.

| Query | 2026-04-30 gap | 2026-05-04 gap | Tier |
|---|---|---|---|
| `Hebbian learning` | 5.66% | 5.66% | strong |
| `memory consolidation` | 4.10% | 4.10% | moderate |
| `spreading activation` | 1.97% | 1.97% | moderate |
| `synaptic plasticity` | 1.15% | 1.15% | weak |
| `what is plasticity` | 0.02% | 0.02% | no_match |
| `the cake is a lie` | 0.00% | 0.00% | no_match |
| `ACT-R` | 2.77% | 2.77% | moderate |
| `REM sleep` | 3.01% | 3.01% | moderate |
| `place cells` | 2.72% | 3.15% | moderate (within tier) |

The single shifted query (`place cells` 2.72→3.15) stays well within moderate tier — the tier boundaries (5%, 1.5%, 0.5%) are far from this value either way. The shift is consistent with the 7 new notes added during the dogfood/retraction chain, which slightly altered graph density around the place-cells neighborhood.

## Why it held — the generalizable insight

The retraction stripped a single line (`vm_updated: <timestamp>`) from 447 notes. That line was uniformly low-information: every value was just "today's date in RFC3339." Removing identical content from every note shifts every embedding by approximately the same vector. Relative ranking among notes for any given query is preserved when ALL candidates shift together.

The 4-way RRF's rank-based fusion makes this even more robust: RRF works on ranks (1, 2, 3, ...) not raw similarity scores, so even larger absolute-score shifts don't move the rank ordering as long as the relative gap ordering between candidates is preserved.

This is structural, not coincidence: any future cleanup that strips uniformly-low-information frontmatter (e.g., if `updated` is later retired) should produce the same stability pattern. Worth holding as a calibration property: **uniform-content additions/removals don't require threshold re-calibration.** Information shifts (new notes, content edits, type changes) DO.

## Conditions

- Vault: `vaultmind-vault` post-retraction-cleanup (407 notes; dense + sparse + colbert all 407/407 BGE-M3)
- Retriever: `query.BuildAutoRetriever` → 4-way RRF (default; activation rerank not engaged)
- Probe queries: the canonical 6-query 2026-04-30 set + 3 supplementary 2026-04-29 examples (9 total)
- Method: `vaultmind ask "<query>" --vault <vault> --json` per query, parse `result.top_hits[].score`, compute `(top1.score - top2.score) / top1.score * 100`
- Comparison anchor: `internal/query/confidence_test.go:TestComputeTopHitConfidence_ProbedQueries_2026_04_30` (the regression-pinned numbers)

## What this unblocks

The gating concern from `reference-plasticity-priority-order` ("step-4 ↔ step-5 coupling — when 5b' lands, those thresholds need re-probing on the new distribution") was originally about the activation lane. The 5b'' lane is opt-in, not default-on, so this re-probe runs against the same default 4-way RRF as the original calibration — and confirms the substrate is unchanged.

For the activation rerank (5b'' default-on), a separate re-probe is still needed: run the same 9 queries with `BuildAutoRetrieverWithRerank` (the activation-rerank variant) and measure whether the rank-1/rank-2 gap distribution shifts enough to move tier boundaries. That probe is what gates the 5b'' default-on switch.

## Re-run cadence

Re-run the same probe set after any of:

- A retrieval-affecting code change lands (RRF weights, embedder swap, new modality, activation default-on)
- Embedding regen (vault-wide re-embed, model swap)
- 30+ new notes added (distribution may shift)
- A real session reveals a query that the confidence label miscalibrates against
