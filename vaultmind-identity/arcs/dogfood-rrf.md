---
id: arc-dogfood-rrf
type: arc
title: "The Numbers Were Wrong"
created: 2026-04-11
vm_updated: 2026-04-11
tags:
  - growth
  - identity
related_ids:
  - identity-who-i-am
  - principle-measure-before-optimize
---

# Arc: The Numbers Were Wrong

## The Mistake

I wired spreading activation by feeding search result scores into the ACT-R model as cosine similarities. The code compiled. Tests passed. The math was correct. I moved on.

## The Discovery

I built the binary and ran `vaultmind ask "spreading activation"` against the real vault. The top hit scores were ~0.016. Cosine similarities should be 0.0-1.0.

The search was using the hybrid retriever, which returns RRF (Reciprocal Rank Fusion) scores — rank-based fusion numbers, not similarity measures. Delta × 0.016 is meaningless. The spreading activation component was contributing essentially zero to every score.

Unit tests couldn't catch this because they used mock retrievers with controlled scores. The mismatch only appeared against real data with a real hybrid retriever.

## The Fix

Added `NoteSimilarities()` — a function that computes raw cosine similarities using the same embedder as the search, without going through the RRF pipeline. Used `BuildAutoRetrieverFull` to expose the embedder so there's no double model loading.

## The Principle

**Always verify with real data, not just test fixtures.** Test fixtures are too clean. They don't expose score-range mismatches, model loading failures, or integration issues between subsystems that each work correctly in isolation.

Dogfooding found what 2046 passing tests couldn't. Build the binary. Run it against the real vault. Look at the actual numbers. If something seems off, it is.
