---
id: concept-free-energy-principle
type: concept
title: Free Energy Principle
created: 2026-04-26
vm_updated: 2026-04-26
tags:
  - neuroscience
  - computational-neuroscience
  - active-inference
  - bayesian-brain
related_ids:
  - concept-predictive-coding
  - concept-default-mode-network
  - concept-hebbian-learning
source_ids:
  - source-friston-2010
---

## Overview

The **free energy principle (FEP)** is Karl Friston's proposal that any self-organising system that maintains a non-equilibrium steady state with its environment must, in effect, minimise an information-theoretic quantity called *variational free energy* — an upper bound on the system's "surprise" (negative log probability of its sensory observations under its own internal model). Brains, on this account, are not many computational systems doing many things, but one computational system doing one thing: minimising free energy by either updating beliefs (perception, learning) or acting to make the world conform to predictions (action).

The most widely cited statement of the principle is [[source-friston-2010|Friston (2010)]] in *Nature Reviews Neuroscience*, but the framework spans dozens of papers since 2005 and continues to evolve into the broader programme of *active inference*.

## How It Works

The FEP starts from a Markov-blanket formalisation of "system" and "environment". The system has internal states, sensory states (its inputs), and active states (its outputs). It cannot directly observe environmental states, only sensory ones. To persist as a recognisable thing, the system must keep its sensory states within a viable distribution.

Doing this is mathematically equivalent to minimising variational free energy:

`F = E_q[log q(s) - log p(o, s)]`

where `o` is sensory observations, `s` is hidden environmental states, and `q(s)` is the system's recognition density (its beliefs about hidden states). Minimising `F`:

- with respect to the recognition density `q` → perception, inference, learning (update beliefs to match data).
- with respect to action → active inference (act so that data match beliefs).

[[predictive-coding|Predictive Coding]] is the most prominent neural-coding instantiation: prediction errors at each cortical level are precisely the message-passing signals that gradient-descend on free energy.

## Key Findings

- **Unification:** Friston shows formal equivalences between FEP and the Bayesian brain, predictive coding, optimal control, neural Darwinism, infomax, and exploration-exploitation trade-offs, all as special cases of free-energy minimisation under different generative models ([[source-friston-2010|Friston 2010]]).
- **Active inference:** Action selection drops out of the same objective when extended to *expected* free energy, which decomposes into pragmatic value (goal-directed reward) and epistemic value (information gain / uncertainty reduction).
- **Precision weighting:** The relative weighting of priors vs. likelihoods is implemented neurally as gain modulation (precision), with neuromodulators (dopamine, acetylcholine, noradrenaline) tuning it.
- **Empirical applications:** Active-inference models have been fit to interoceptive perception, motor control, decision-making under uncertainty, and clinical populations (psychosis, autism, depression).

## Recent Developments

- **Active inference textbook (Parr, Pezzulo & Friston, 2022):** A formal consolidation of the framework, presenting both the discrete (POMDP-style) and continuous-time formulations.
- **Empirical-vs-mathematical FEP debate:** A long-running discussion (Andrews 2021; Bruineberg et al. 2022) about whether FEP is a substantive scientific claim or a tautological mathematical truism. The current consensus is that FEP-as-principle is mathematical; it is FEP plus a particular generative model that becomes empirically falsifiable.
- **FEP and machine learning:** Free-energy / variational objectives underlie modern variational autoencoders, normalising flows, and certain self-supervised methods, giving the principle a strong engineering footprint.
- **Interoception and self-modelling:** Seth, Barrett, and others have extended FEP to interoceptive perception, emotion, and the construction of the self — most influential outside cognitive neuroscience proper.

## Connections

The FEP is the high-level umbrella under which [[predictive-coding|Predictive Coding]] sits as a cortical implementation. Both share a substrate of [[hebbian-learning|Hebbian Learning]] (for learning the generative model) and predict that resting / offline activity in the [[default-mode-network|Default Mode Network]] reflects the brain running its generative model in the absence of input.

For VaultMind, the actionable take-away is operational, not metaphysical: agent behaviour can be framed as reducing uncertainty about a model of the world, and retrieval is one tool for doing so. Choosing what to retrieve = choosing what would most reduce expected free energy. This is a principled grounding for "epistemic" retrieval (fetch what surprises the agent's current model) over purely "pragmatic" retrieval (fetch what matches the query string).
