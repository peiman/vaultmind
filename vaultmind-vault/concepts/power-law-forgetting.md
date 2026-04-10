---
id: concept-power-law-forgetting
type: concept
title: "Power-Law Forgetting"
status: active
created: 2026-04-09
tags:
  - memory
  - forgetting
  - mathematical-model
related_ids:
  - concept-forgetting-curve
  - concept-base-level-activation
  - concept-hebbian-learning
  - source-anderson-schooler-1991
---

## Overview

Forgetting follows a power law R(t) = a * t^(-b), not the exponential Ebbinghaus originally proposed. Wixted and Ebbesen (1991) demonstrated this across multiple experimental paradigms. The critical implication: forgetting slows down over time. A memory that survives the first hour has a much better chance of surviving the next day than a fresh memory does of surviving the first hour.

This matches ACT-R's base-level activation equation, which uses the same power-law form. It also explains why spaced repetition works: each retrieval restarts the decay curve from a higher baseline.

## The Mixture-of-Exponentials Explanation

At the single-synapse level, LTP decay is approximately exponential — early-phase LTP decays in hours, late-phase LTP persists for weeks. The population-level power law emerges from the superposition of many exponential decay processes with different time constants (Anderson & Tweney, 1997). This bridges single-synapse neuroscience to behavioral forgetting curves without requiring any single synapse to follow a power law.

Fusi, Drew, and Abbott (2005) formalized this with a cascade model: synapses exist in multiple states with different transition rates, producing power-law memory lifetimes from simple exponential transitions at each state. This is the most direct neural mechanism explaining power-law forgetting.

## Key Properties

- Power law: R(t) = a * t^(-b) — forgetting decelerates with time
- Jost's Law (1897): Of two memories of equal strength, the older one will decay more slowly
- Consolidation gradient: Recently formed memories are more fragile; older memories have been stabilized
- Environmental match: The power-law form matches the statistical distribution of information recurrence in natural environments (Anderson & Schooler, 1991)

## Implications for VaultMind

The base-level activation equation `Bi = ln(sum(tj^(-d)))` already captures power-law forgetting. When implementing activation-based scoring:

- The decay parameter `d` controls how fast notes fade (typically d ~ 0.5)
- Each access adds a new term to the sum, boosting activation
- Old accesses contribute less but never reach zero — a note accessed a year ago still has some residual activation
- This naturally implements "notes you use a lot stay accessible; notes you forget about fade but don't disappear"

## Sources

- Wixted, J.T. & Ebbesen, E.B. (1991). On the form of forgetting. *Psychological Science*, 2(6), 409-415. DOI: 10.1111/j.1467-9280.1991.tb00175.x
- Anderson, R.B. & Tweney, R.D. (1997). Artifactual power curves in forgetting. *Memory & Cognition*, 25(5), 724-730. DOI: 10.3758/BF03211315
- Fusi, S., Drew, P.J., & Abbott, L.F. (2005). Cascade models of synaptically stored memories. *Neuron*, 45(4), 599-611. DOI: 10.1016/j.neuron.2005.02.001
- Wixted, J.T. (2004). On common ground: Jost's (1897) law of forgetting and Ribot's (1881) law of retrograde amnesia. *Psychological Review*, 111(4), 864-879. DOI: 10.1037/0033-295X.111.4.864
