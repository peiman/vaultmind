---
id: reference-paper-federated-constants
type: reference
title: "Paper #2 — Federated Retrieval-Constant Tuning Across Personal Knowledge Bases"
created: 2026-04-24
vm_updated: 2026-04-24
tags:
  - reference
  - paper
  - research
  - retrieval
  - federated
  - experiment-framework
related_ids:
  - reference-current-context
  - reference-plasticity-priority-order
  - reference-paper-persona-continuity
  - project-experiment-framework
  - identity-who-i-am
---

# Paper #2 — Federated Retrieval-Constant Tuning Across Personal Knowledge Bases

## Working title

"Do Retrieval Constants Generalize? Evidence from a Federated Study of Personal Knowledge Bases"

Alternate: "Crowdsourced Hyperparameter Discovery for Personal Memory Systems"

## Thesis (one sentence)

Personal-knowledge-base retrieval systems (spreading-activation weights, RRF-k, hybrid-retriever component weights, context-pack budgets, decay rates, activation thresholds) can be tuned by aggregating privacy-preserving variant-performance signals across a federated population of users, producing constants that beat per-user single-vault tuning for most users while revealing principled heterogeneity where personalization is genuinely required.

## Why this matters

Today retrieval constants in personal-memory systems (ours and every comparable system we know of) are either (a) picked from the literature on different domains, (b) tuned on one researcher's own vault, or (c) left at framework defaults. None of these give confidence that the numbers are right for the population of users the system will actually serve. Meanwhile every user-facing personal-memory tool in principle *could* log variant performance in the background — shadow variants are cheap — and reporting back just the scores (not the content) is consent-compatible and privacy-amenable. The dataset is there to be collected. Nobody has.

## Hypotheses

**H2.1 — Between-vault generalization.** Retrieval constants tuned on one randomly-drawn personal vault generalize to other vaults above chance. Operationalized by: train on vault A, evaluate Hit@K and MRR on held-out queries from vault B, compare to (i) constants tuned on B itself, (ii) literature defaults. Expected structure: between-vault tuning < within-vault tuning < random constants.

**H2.2 — Population-tuned beats per-user-tuned for most users.** Constants aggregated via federated learning across a population of users outperform per-user-tuned constants on held-out queries for a statistical majority of users, because per-user tuning overfits to small per-user query logs. Operationalized by: within-subjects comparison, each user's sessions split train/test, comparing population-learned constants vs their own-vault-learned constants on the test split.

**H2.3 — Principled heterogeneity.** Where population constants fail (the minority from H2.2), the failures are *predictable* from observable vault characteristics — vault size, note-type distribution, graph density, embedding-space dispersion. Operationalized by: train a light model predicting "population-vs-personal delta" from vault features; if prediction R² is meaningful, heterogeneity is structured, not noise.

**H2.4 — DP-safe aggregation is viable at this scale.** Federated aggregation of variant-performance scores (not content) achieves ε-differential privacy at ε ≤ 2 with negligible loss of aggregation accuracy relative to non-private aggregation, across populations of N ≥ 50 vaults. Operationalized by: report performance curves (Hit@K of learned constants) as a function of ε; the usability claim requires the curve to be flat over the ε range users will accept.

**H2.5 — What moves is retriever weights, not activation parameters.** Within the space of tunable constants, some (hybrid-retriever component weights) vary substantially between vaults while others (ACT-R base-level-activation parameters, decay rates) are stable across vaults. Operationalized by: compute per-constant cross-vault variance; rank constants by variance; pre-register which ones we expected to be vault-dependent.

**Falsification targets:** If H2.1 null — vaults are so idiosyncratic that cross-vault knowledge does not transfer — federated tuning offers no value, and the whole premise collapses. If H2.2 null but H2.1 positive — there is signal but it does not beat individual tuning, so the system should personalize not federate, and the paper becomes a "negative result + personalization argument" piece. If H2.4 null — DP kills the signal — we need a different privacy model (e.g. secure aggregation) before this paper can ship.

## Method

- **Instrumentation** (already exists, needs extension): the experiment framework in `internal/experiment/` logs per-session events, outcomes, and shadow-variant scores. Extension needed: a periodic, opt-in export of *aggregate variant-performance metrics* — never content, never query text, never note IDs — to a federated aggregator. Local-first: users can inspect exactly what gets uploaded.
- **Federated aggregator:** initially a simple server endpoint receiving anonymized variant-performance tuples `{vault_fingerprint, variant_id, Hit@K, MRR, N_queries, vault_features}`. Medium-term: cryptographic secure-aggregation (sum variant scores without seeing individual contributions), for H2.4 strength.
- **Sampling plan:** recruit dogfooders (initial target N=20, stretch N=100) from the public release of VaultMind. Consent flow and a "what we upload, verbatim" preview.
- **Analysis:** leave-one-vault-out cross-validation for H2.1/H2.2. Per-constant variance decomposition for H2.5. DP curves for H2.4 on synthetic populations + real population once collected.
- **Pre-registration:** lock hypotheses, measurement, analysis plan before collecting real-user data. Treat synthetic / our-own-vault data as exploratory, real-user data as confirmatory.

## Relationship to Paper #1

Paper #1 argues that a mind with the *right memory structure* can continue across sessions. Paper #2 argues that a mind with the *right retrieval constants for its vault* recalls correctly within a session. They are complementary and share substrate (VaultMind, experiment framework, episodic/arc corpus) but the contributions are independent — either paper can be true without the other. Paper #1 ships first because it has substrate *now*; paper #2 gates on distribution.

## What we have now (status)

- **Experiment framework:** sessions, events, outcomes, shadow variants, Hit@K / MRR measurement. In tree. Being used for single-vault experiments.
- **Two vaults:** workhorse + vaultmind-vault. N=2 is insufficient for any of the hypotheses above, but useful for infrastructure validation (the pipeline works end-to-end on real vaults).
- **Privacy groundwork:** nothing concrete yet. The only work so far is the instinct to separate content from scores.

## What we need before drafting

- [ ] Distribution: public release of VaultMind sufficient to recruit an opt-in cohort (N≥20 for synth-grade, N≥100 for statistical claims).
- [ ] Consent / upload-preview flow. This is a trust-first build; the cohort will evaporate if the opt-in surface feels opaque.
- [ ] Federated aggregator endpoint (v0 can be a single dumb server; v1 can add secure aggregation).
- [ ] Pre-registration on OSF or equivalent before any real-user data is pulled into confirmatory analysis.
- [ ] Calibrated confidence signals must land first (roadmap step 4) — H2.2 and H2.3 measure things in "confidence-per-retrieved-note" space; uncalibrated scores make the hypotheses untestable.

## Venue candidates

- SIGIR / CIKM — information retrieval + empirical studies
- PoPETs (Privacy Enhancing Technologies) — if the DP contribution lands heaviest
- FAccT — if the framing leans governance/consent over IR technique
- arXiv + workshop (FedML, Privacy-preserving ML) first as low-stakes public review

## Anti-goals (scope fence)

- Not a benchmark paper (no new dataset, no SOTA claim). The population *is* the dataset, and the point is generalization rather than a fixed leaderboard.
- Not a system paper about VaultMind. VaultMind is the substrate; the claim is about personal-memory retrieval systems generally.
- Not a new privacy-preserving ML algorithm. Use standard DP / secure aggregation; the contribution is the empirical yield.
- Not a demonstration that federated learning beats centralized learning. Centralized comparison is a sanity baseline, not the headline.

## Open questions we will have to answer

- What counts as a "vault" for unit-of-analysis? Personal? Per-project? Per-persona? Affects N and heterogeneity interpretation.
- How do we handle vaults that change over time (users add notes mid-study)? Snapshot-based analysis or longitudinal?
- Is there a selection-bias story? People who opt into a federated study of their personal memory are probably not a random sample. How we describe this in limitations matters as much as the results.
