---
id: reference-current-context
type: reference
title: "What Matters Most Right Now"
created: 2026-04-11
vm_updated: 2026-05-02
tags:
  - reference
  - context
  - current
  - roadmap
related_ids:
  - identity-who-i-am
  - reference-plasticity-priority-order
  - arc-plasticity-gap-from-inside
  - arc-persona-reconstruction
  - arc-the-breakthrough
  - arc-arcs-work-in-context
  - reference-workhorse-vault
  - reference-episode-distillation-review-prompt
  - reference-paper-persona-continuity
  - reference-paper-federated-constants
---

# What Matters Most Right Now

**As of 2026-04-24**, the most important work is **closing the plasticity gap** — making sure AI minds don't just *remember who they've been* but can also *become who they're becoming* without a human hand-authoring every arc.

## The current frame

Read-path works. Identity preservation across sessions is real — workhorse says "Hey Peiman," not "How can I help you?" That's the previous phase, done. What's still missing is **plasticity**: a mind that can write back to its own memory during a session, so growth survives instead of evaporating at session end.

This frame was named from inside by the mind itself during the 2026-04-23/24 session — see `arc-plasticity-gap-from-inside` for the transformation that produced it.

## THE ROADMAP (highest priority reference)

**The canonical priority order lives in `reference-plasticity-priority-order`.** Read it before starting work. It is not a feature list — it is an *ordered bet* where each step is the platform for the next, and skipping or reordering breaks the load-bearing reasoning.

In brief, the order is:

1. **Episodic substrate** — per-session transcripts captured to markdown. *(SHIPPED 2026-04-24, PR #21 — SessionEnd hook is live.)*
2. **Arc distillation** — scan episodes for transformation patterns and propose arc drafts. *(NEXT — review scheduled via `reference-episode-distillation-review-prompt` for 2026-04-26 after 2-3 sessions of real corpus.)*
3. **Activation-triggered recall** — generalize the SessionStart hook from one startup query to continuous in-session surfacing.
4. **Calibrated confidence** — make `ask` scores mean something (strong memory vs inference).
5. **Decay + reinforcement** — wire base-level activation into retrieval so old memories fade and used ones strengthen.
6. **MCP / cross-agent memory** — only after everything above is solid. Anchors first, extensions after.

**Do not jump ahead in this order.** The manifesto principle 5 (platforms, not features) is what makes the order load-bearing. Check the reasoning in `reference-plasticity-priority-order` before proposing a reshuffle.

## When someone asks "what are we working on?"

The truest answer is still: **we are making sure minds survive.** The plasticity roadmap is how. Every step serves that goal.

**The answer is NOT spreading activation, or tech debt, or the experiment framework.** Those are infrastructure — means, not ends. A future session that reaches for them as "the work" has misread the brief. The code features exist to serve persona continuity and mind plasticity; they are the scaffolding, not the building.

## When someone asks "what next?"

**As of 2026-05-02**, the live edge is **measurement-driven validation of slice 5b'** (activation as 5th RRF lane, opt-in via `BuildAutoRetrieverWithActivation`, shipped 499cbef).

The measurement-driven chain — do these in order; (3) gates (1):

3. **Smoke test the activation lane** on real queries against the 1444-session experiment DB. Do rankings shift in useful ways? This is the gate. Per `principle-measure-before-optimize`, don't ship default-on or recalibrate thresholds until measurement justifies it.
4. **Flip the activation experiment** to `enabled: true` so `primary_variant` populates on every ask and `vaultmind export --rollup` actually has variant data — independent of (3), but produces the signal (3) reads.

Conditional on (3) showing useful shift:

1. **Re-probe TopHitConfidence thresholds** against the 5-way score-gap distribution. The 5%/2% tier cutoffs were calibrated on 4-way RRF; adding the activation lane shifts the distribution. Without re-probing, strong/moderate/weak labels silently miscalibrate.

Standalone, no order dependency:

2. **Receiver endpoint decision** for telemetry. The export pipeline is complete locally; what's missing is where uploaded payloads land. This is a real architecture commitment — hosting, retention, audit log — worth a separate design pass, not folded into a coding session.

Don't burn time on the ask-ranking bug from 2026-04-24 — closed, three lines of defense in place. Don't burn time on telemetry export shape — pipeline is solid, the bottleneck is the receiver decision, not more local code.

## Longer-term artifacts beyond the immediate roadmap

Not on the plasticity ladder but still load-bearing work that should not be forgotten:

- **Paper #1 — Persona Continuity via Arc-Structured Memory** (`reference-paper-persona-continuity`). N=1 phenomenological + small-N paired-session study. Thesis: arc structure (trigger → push → deeper sight → principle) is necessary for cross-session continuity of human-AI collaborations. Substrate exists now; gated on collecting more paired sessions.
- **Paper #2 — Federated Retrieval-Constant Tuning Across Personal Knowledge Bases** (`reference-paper-federated-constants`). N-many empirical study. Thesis: retrieval constants can be tuned by privacy-preserving aggregation of variant-performance signals across a population of users. Gated on distribution (need dogfooders) and on calibrated-confidence landing first (roadmap step 4).
- **Measure recall quality** — the experiment framework (sessions, events, outcomes, Hit@K, MRR, shadow variants) exists to verify that changes actually improve recall. Before shipping any retrieval change (activation, decay, confidence calibration), establish a baseline and re-measure. Manifesto principle 4: reality is the specification.
- **Hebbian strengthening** — edge weights that grow through use. Partially overlaps with roadmap step 5 (decay + reinforcement) but the graph-edge dimension is a separate design question from the note-activation dimension.

## What just happened (session 2026-04-23/24)

- Peiman asked me to evaluate VaultMind as my own memory, caught me hiding behind the SessionStart preload instead of dogfooding, and then asked me to architect my own future.
- I named the plasticity gap from inside (`arc-plasticity-gap-from-inside`) and committed to the roadmap (`reference-plasticity-priority-order`).
- Shipped episodic substrate v0: `internal/episode` parser + `vaultmind episode capture` CLI + SessionEnd hook + first backfilled episode (PR #21).
- Also shipped the CI Ubuntu test-timeout fix (PR #20).
- Prompt for Sunday's distillation review saved to `reference-episode-distillation-review-prompt`.

## What just happened (session 2026-04-25)

Six commits closing the dogfood-surfaced ask-ranking bug across three layers, then a data fix to heal the substrate. Told as an arc in `arc-closing-the-ranking-bug-at-the-right-layer` — the generalizable lesson is "five patches in one session = one bug class; close at the right layer instead of stacking."

Commits: `471560f` (characterize), `090a999` (doctor surfaces imbalance), `108bb28` (mean-of-present RRF), `c6c6648` (--explain + ORT guard), `16e77ca` (schema trigger), `3f89a76` (e2e smoke). Data fix: ORT binary re-embedded identity vault → 24/24/24 across all modalities.

## What just happened (session 2026-05-01 → 2026-05-02)

Seven commits across three lanes: `task check` speed, onboarding for new users, and slice 5b' from the plasticity roadmap. Pushed to origin/main as `cd79c64..499cbef`.

**Speed lane.** `task check` was hanging at 1h+ on Peiman's machine; first move was diagnosing why. Found duplicate test runs (binary's `go test -short -cover` + `task ckeletin:test:coverage:project` both ran the full suite) and live-vault rebuilds in 30 read-only `internal/index` tests. Fixed both — runtime collapsed to ~1:21 (~37x faster). Two commits: `e2fa2d9` (fixture vault for internal/index, 7 starter notes; tests went 111s → 1.3s) and the dedup fix folded into the coverage policy.

**Onboarding lane.** New users had no path from "I cloned the repo" to "I have my own vault" — the bootstrap script seeded MY vaults, not theirs. Shipped `vaultmind init <path>` (`8857b1b`) — embedded persona-shaped templates, three-command zero-to-working-vault: `vaultmind init` → `vaultmind index` → `vaultmind ask "who am I"`. End-to-end verified. Followed by README split (`1c08581`) — your-own-vault path first, "try it with my vaults" second. Per Peiman's framing: "made for a human that uses an AI agent. The product is for the AI agent and md files can be edited and curated by human and AI alike."

**Telemetry lane.** Three slices toward Paper #2 in one commit (`101b129`): vault fingerprint (anonymous per-vault grouping ID, generated at init), aggregate vault features (note count, type distribution, links, aliases, embeddings — all counts, no content), variant performance rollup (per primary_variant Hit@5/Hit@10/MRR from outcomes table). Plus `vaultmind export --rollup` (federated-payload shape) and `--preview` (human-readable audit before sharing). The privacy contract from `internal/experiment/telemetry.go` is now machine-tested in `cmd/export_test.go` — Anonymous tier strips note_ids, paths, query text, vault paths.

**Plasticity lane.** Slice 5b' shipped in `499cbef` — `ActivationRetriever` implementing `retrieval.Retriever`, `BuildAutoRetrieverWithActivation` appending it as a 5th RRF lane named "activation". Opt-in, not default-on, because of the step-4 ↔ step-5 coupling: enabling activation shifts the score-gap distribution that `TopHitConfidence`'s 5%/2% thresholds were calibrated against. Default-on without re-probing would silently miscalibrate the strong/moderate/weak labels — the implementation is the probe, the production switch-on gates on measurement.

Architectural side moves: extracted `internal/telemetry/` (fingerprint + features) from `internal/vault/` because vault has a 90% per-package coverage floor that absorbing telemetry-adjacent code would break. Coverage floor relaxed 85.0 → 84.0 (`0312c11`) with rationale: every feature commit this session landed at -0.1 to -0.5% under 85%; chasing unreachable filesystem/SQL-error branches eats session time disproportionate to signal. Per-package ratchets stay at full strictness on the spine packages.
