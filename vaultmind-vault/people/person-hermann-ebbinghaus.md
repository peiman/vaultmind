---
id: person-hermann-ebbinghaus
type: person
title: "Hermann Ebbinghaus"
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Ebbinghaus
tags:
  - cognitive-science
  - memory-models
  - forgetting
related_ids:
  - concept-forgetting-curve
  - concept-spacing-effect
  - source-ebbinghaus-1885
url: "https://en.wikipedia.org/wiki/Hermann_Ebbinghaus"
---

## About

Hermann Ebbinghaus was a German psychologist who conducted the first systematic experimental studies of memory on himself. His 1885 monograph introduced both the [[Forgetting Curve]] — the exponential decay of retention over time without rehearsal — and the [[Spacing Effect]], the finding that distributed practice produces far stronger long-term retention than massed repetition.

## Key Contributions

Ebbinghaus's forgetting curve is the empirical foundation for any time-decay weighting in VaultMind's retrieval scoring. The curve's shape (rapid initial loss, slower asymptote) justifies why `vm_updated` alone is insufficient — a note accessed once long ago should score differently from one accessed repeatedly. His spacing effect underlies the design principle that VaultMind should surface older but relevant notes proactively rather than waiting for explicit recall, keeping the memory trace from decaying past the retrieval threshold.
