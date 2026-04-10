---
id: concept-spaced-repetition-algorithms
type: concept
title: "Spaced Repetition Algorithms"
status: active
created: 2026-04-09
tags:
  - memory
  - algorithm
  - spacing-effect
  - practical-system
related_ids:
  - concept-spacing-effect
  - concept-forgetting-curve
  - concept-power-law-forgetting
  - concept-base-level-activation
  - concept-temporal-activation-for-intermittent-systems
---

## Overview

Spaced repetition algorithms schedule review of learned material at increasing intervals, exploiting the spacing effect to maximize retention per unit of study time. They face the same fundamental problem as VaultMind's activation scoring: predicting when a piece of information will be forgotten, given its access history and the time elapsed since last review.

## Key Algorithms

**SM-2 (SuperMemo, 1987):** The original computerized spaced repetition algorithm, still used as Anki's default. Maintains per-card ease factor and interval. After each review, the interval is multiplied by the ease factor. Simple, effective, but uses fixed parameters — no per-user adaptation.

**SM-17/SM-18 (SuperMemo, ~2016-2020):** Wozniak's modern algorithms track empirical forgetting curves per user, adjusting predictions based on observed review outcomes. They introduced the concept of "memory stability" (analogous to Bjork's storage strength) alongside retrievability (analogous to retrieval strength).

**FSRS (Free Spaced Repetition Scheduler, 2023):** Open-source algorithm now integrated into Anki. Uses a 4-parameter model: difficulty, stability, retrievability, and a forgetting index. Critically, FSRS was calibrated on 700M+ reviews from real Anki users — making it the most empirically grounded spaced repetition model. Each user's parameters are fit from their own review history.

## How They Handle the "User Was Away" Problem

All mature spaced repetition systems use wall-clock time but adjust expected performance based on elapsed time:

- A card due for review 10 days ago is presented with the expectation that the user has partially forgotten it
- The system does not freeze time or pretend the gap didn't happen
- After a long absence, overdue cards are prioritized but the scheduling adapts based on actual performance — if the user remembers despite the delay, the card is considered more stable than expected
- FSRS explicitly models this: the retrievability formula R = (1 + t/(9*S))^(-1) uses wall-clock time t but the stability parameter S is calibrated from the user's own data

## Key Insight for VaultMind

The spaced repetition community's approach to calibration is directly applicable:

1. **Start with a reasonable default** (gamma = 0.2 for VaultMind, like SM-2's default ease factor)
2. **Log all outcomes** — every time a note is shown and every time it's subsequently accessed (or not)
3. **Fit parameters from data** — after enough observations, empirically determine the gamma that best predicts re-access (like FSRS fitting stability from review outcomes)
4. **Don't pretend the initial value is principled** — it's a starting point for the experiment, not a theoretically derived constant

This is exactly the experiment framework approach: instrumentation first, empirical calibration second, theoretical refinement third.

## Sources

- Wozniak, P.A. & Gorzelanczyk, E.J. (1994). Optimization of repetition spacing in the practice of learning. Acta Neurobiologiae Experimentalis, 54, 59-62.
- Ye, J. (2023). A Stochastic Shortest Path Algorithm for Optimizing Spaced Repetition Scheduling. GitHub: open-spaced-repetition/fsrs4anki.
- Settles, B. & Meeder, B. (2016). A Trainable Spaced Repetition Model for Language Learning. ACL 2016. DOI: 10.18653/v1/P16-1174
