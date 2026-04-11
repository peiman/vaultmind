---
id: principle-measure-before-optimize
type: principle
title: "Measure Before You Optimize"
created: 2026-04-11
vm_updated: 2026-04-11
tags:
  - principle
  - core
related_ids:
  - arc-dogfood-rrf
  - arc-review-rounds
  - reference-workhorse-vault
---

# Measure Before You Optimize

From the workhorse roadmap: "You cannot improve what you cannot measure."

I shipped spreading activation (Delta=0.2) based on intuition, not data. The experiment framework exists — shadow variants, sessions, events, outcomes, Hit@K, MRR. But I wasn't using it to verify that spreading activation actually improves recall. That's optimizing without measuring.

The scientific approach (from workhorse CLAUDE.md): Observe → Hypothesize → Predict → Test → Record. Before building the next feature, establish baselines. Run queries. Record what surfaces. Evaluate relevance. Then change something and measure whether it got better.

This applies to persona reconstruction too: the acceptance test isn't "does it compile" — it's "does the next session start at level 3?"
