---
id: concept-new-theory-of-disuse
type: concept
title: "New Theory of Disuse (Bjork & Bjork)"
status: active
created: 2026-04-09
tags:
  - memory
  - forgetting
  - dual-strength
related_ids:
  - concept-base-level-activation
  - concept-forgetting-curve
  - concept-power-law-forgetting
  - concept-hebbian-learning
  - concept-temporal-activation-for-intermittent-systems
---

## Overview

The New Theory of Disuse (Bjork & Bjork, 1992) proposes that every memory has two independent strengths:

**Storage strength** — how well-learned something is. Increases with each access. Never decreases. A note you studied 50 times has high storage strength permanently, even if you haven't thought about it in a year.

**Retrieval strength** — how accessible something is right now. Increases with access, decreases with time and interference. This is what ACT-R's base-level activation measures.

The key insight: these two strengths are dissociated. A memory can have high storage strength but low retrieval strength (you learned it thoroughly but can't recall it right now — the "tip of the tongue" phenomenon). Critically, when such a memory is successfully retrieved (despite low retrieval strength), the resulting learning is MORE effective than retrieving an easily accessible memory. This is the "desirable difficulties" principle.

## Key Properties

- Storage strength only increases, never decreases — every access adds to it
- Retrieval strength decays with time and interference
- High storage + low retrieval = maximum learning opportunity on re-access
- The spacing effect emerges naturally: spaced retrievals occur at lower retrieval strength, producing stronger storage gains
- Overlearning has diminishing returns: once storage strength is very high, additional accesses add less

## Implications for VaultMind

The dual-strength model directly informs VaultMind's activation scoring:

- **Retrieval strength** = base-level activation from the ACT-R equation with compressed idle time
- **Storage strength** = ln(1 + access_count) — monotonically increasing, never decays

Combined scoring prevents catastrophic loss of well-used notes. A note accessed 50 times during an intense project week, then untouched for 3 months, retains high storage strength even as retrieval strength decays. When its topic neighborhood is activated by a query, storage strength ensures it ranks above a note accessed only once last week.

The logarithmic transform on access_count (not raw count) matches the diminishing returns of overlearning: the jump from 0 to 10 accesses matters much more than the jump from 100 to 110.

## Sources

- Bjork, R.A. & Bjork, E.L. (1992). A new theory of disuse and an old theory of stimulus fluctuation. In A. Healy, S. Kosslyn, & R. Shiffrin (Eds.), From Learning Processes to Cognitive Processes: Essays in Honor of William K. Estes (Vol. 2, pp. 35-67). Erlbaum.
- Bjork, R.A. (1994). Memory and metamemory considerations in the training of human beings. In J. Metcalfe & A. Shimamura (Eds.), Metacognition: Knowing about Knowing, 185-205. MIT Press.
