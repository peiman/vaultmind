---
id: source-anderson-schooler-1991
type: source
title: "Anderson & Schooler. Reflections of the Environment in Memory (1991)"
created: 2026-04-03
vm_updated: 2026-04-03
url: "https://doi.org/10.1111/j.1467-9280.1991.tb00174.x"
aliases:
  - Anderson Schooler 1991
  - Environmental Memory paper
tags:
  - cognitive-science
  - memory-models
related_ids:
  - concept-base-level-activation
  - concept-forgetting-curve
---

# Anderson & Schooler — Reflections of the Environment in Memory (1991)

Anderson and Schooler analyzed the statistical structure of real-world information environments—newspaper headlines, parent speech to children, email logs—and found that the probability of needing a piece of information closely tracks the ACT-R base-level activation formula. Specifically, need probability decays as a power function of time and increases with each repetition, matching the shape of [[forgetting-curve|Forgetting Curve]] and spacing-effect data. The memory system, they argued, is not poorly designed; it is optimally adapted to the statistical regularities of the environment.

This rational analysis framing is important because it grounds memory design in ecological validity rather than arbitrary engineering choices. If an information system mirrors the statistical structure that human memory evolved to track, it will feel natural and incur minimal cognitive overhead. The paper also quantified the base-level activation formula's parameters from naturalistic data, giving concrete numerical anchors for [[base-level-activation|Base Level Activation]] calculations.

VaultMind uses the Anderson-Schooler power-law decay constants to calibrate how quickly vault notes lose retrieval priority after their last access. Notes that are revisited frequently are kept highly activated; notes untouched for months decay toward threshold. This means VaultMind's retrieval rankings naturally reflect the user's actual working patterns rather than arbitrary recency cutoffs, reducing noise without requiring manual curation.
