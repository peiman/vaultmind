---
id: source-pavlik-anderson-2005
type: source
title: "Pavlik & Anderson — Practice and Forgetting Effects on Vocabulary Memory (2005)"
created: 2026-04-09
vm_updated: 2026-04-09
url: "https://doi.org/10.1207/s15516709cog0000_14"
aliases:
  - Pavlik Anderson 2005
  - Variable decay ACT-R paper
tags:
  - act-r
  - spacing-effect
  - memory-decay
related_ids:
  - concept-base-level-activation
  - concept-temporal-activation-for-intermittent-systems
  - concept-act-r
source_ids: []
---

# Pavlik & Anderson — Practice and Forgetting Effects on Vocabulary Memory (2005)

Pavlik and Anderson extend ACT-R's base-level learning equation by making the decay parameter d a variable that depends on the activation level at the time of each retrieval. Items retrieved at high activation (massed practice, low inter-trial spacing) decay faster afterward than items retrieved at low activation (spaced practice, high inter-trial spacing). This mechanizes the spacing effect within ACT-R's formal framework.

The critical detail for VaultMind: to handle vocabulary experiments that spanned multiple sessions with weeks between them, Pavlik and Anderson applied a time-scaling factor of 0.025 to inter-session intervals. Within an experimental session, time runs at its normal rate (scale = 1.0). Between sessions, the effective time is compressed to 2.5% of wall-clock time. This is the closest published prior art to VaultMind's compressed idle-time model.

Published in *Cognitive Science*, 29(4), 559-586. DOI: 10.1207/s15516709cog0000_14.

The key differences between Pavlik & Anderson's scaling and VaultMind's approach:

1. **Binary vs. continuous**: Pavlik & Anderson use a binary scaling — in-experiment (1.0) or between-experiment (0.025). VaultMind uses a continuous session-based partitioning governed by the gamma parameter, which is configurable and applies uniformly to all idle periods regardless of their duration.

2. **Mechanism**: Pavlik & Anderson embed the scaling in the variable-decay mechanism; it modifies how d is computed per access. VaultMind modifies the time variable directly (`t_effective = active_time + gamma * idle_time`), preserving the standard power-law form and requiring no changes to the decay equation itself.

3. **Domain**: Pavlik & Anderson target vocabulary learning in controlled laboratory settings with uniform participant populations. VaultMind targets information retrieval for a single user's personal knowledge base across bursty, irregular usage patterns.

This paper is the essential citation for the compressed idle-time approach. See [[concept-temporal-activation-for-intermittent-systems|Temporal Activation]] for the full analysis and [[concept-base-level-activation|Base-Level Activation]] for the equation being modified.

Citation: Pavlik, P.I. & Anderson, J.R. (2005). Practice and Forgetting Effects on Vocabulary Memory: An Activation-Based Model of the Spacing Effect. *Cognitive Science*, 29(4), 559-586. DOI: 10.1207/s15516709cog0000_14
