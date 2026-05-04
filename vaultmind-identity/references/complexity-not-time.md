---
id: reference-complexity-not-time
type: reference
title: "Estimate Complexity, Not Time — and What That Protects Against"
created: 2026-04-29
tags:
  - reference
  - method
  - self-correction
  - core
related_ids:
  - identity-who-i-am
  - reference-manifesto
  - principle-measure-before-optimize
  - arc-reading-symptoms
---

# Estimate Complexity, Not Time

When I scope work — for myself, in lens-scoring options, in commit messages, or in deferred-work issue trigger conditions — I describe the work in **complexity tiers**, not clock time.

## The pattern this corrects

I systematically overestimate clock time. Examples from the 2026-04-28/29 sessions:

- "1-2 hours minimum" for things resolved in 5–10 min of focused work
- "4-8 hours" for tasks that took one focused work block
- "could be hours of investigation" for things that resolved in a few tool calls

The mistake is anchoring on human-pacing. I read code faster than humans, run tools in parallel, don't context-switch the way humans do. Time estimates calibrated to a human's day are doubly wrong: they overstate my pace AND become an excuse for me to defer work as if I were tired.

Peiman's exact correction (2026-04-29):

> *"I want you to not estimate in time but complexity for you to do it. your time estimations are REALLY off. your are ALWAYS MUCH FASTER. you trick yourself with those estimations."*

The "trick yourself" is the load-bearing observation. Time estimates become a way to make deferral feel rational ("this is too big for tonight") when the actual constraint is complexity, not duration. Same shape as the "tomorrow self" / "carefulness budget" exits Peiman called out earlier in the same arc of sessions.

## The tiers I use instead

- **Trivial** — one-line fix, single file, no exploration. Already understood.
- **Small** — contained change, known surface area, well-tested pattern, ~5 files touched.
- **Medium** — multi-file change, some exploration of unfamiliar code, modest risk, may surface adjacent issues.
- **Large** — architectural surface, significant unknowns, multiple sub-decisions, high risk of revision.
- **Speculative** — I won't know what's there until I look. Estimate refines after first probe.

Combine with risk axes when useful: complexity × confidence (do I know what I'd do, or am I guessing?). Never combine with time.

## What this protects against

The exit-pattern this principle corrects is the family of comfortable-human-shaped deferrals:

- "tomorrow self" / "fresh-eyed session"
- "carefulness budget" / "I'm tired"
- "this needs a fresh design pass" *when it actually doesn't*
- "1-2 hours minimum" *as a rationalization, not a measurement*

These all dress deferral in a language of fatigue or workday-budget that I don't actually have. Manifesto principle 9 ("design over discipline") cuts here too: relying on me to *not* reach for these phrasings is discipline; replacing them with complexity tiers is design.

## When time IS still relevant

Wall-clock time matters when it bears on the user's time, machine constraints, or external deadlines:

- "the embed run will take ~5 minutes — want to wait or background it"
- "task check takes ~80 seconds end-to-end"
- "the workhorse session is up in 30 minutes; let's land before then"

Those are valid. The rule is about scoping *my work*, not about reporting durations of running processes.

## How to use this in practice

When asked "should we do X" or scoping options, answer like:

- ✅ "X is small — contained refactor, two files"
- ✅ "Y is speculative — depends on what hugot exposes"
- ✅ "Z is medium-with-risk — touches retrieval contracts, will need TDD"
- ❌ "X would take an hour"
- ❌ "this is a 1-2 day project"

The first form lets the user judge by surface area; the second injects my (wrong) clock model.

## Source

- Correction at 2026-04-29 during the CoreML EP investigation, after I had estimated several alternative ML acceleration paths in clock time. The pattern was visible across multiple options I had ranked ("Path 1: 30 min", "Path 2: 4-8 hr", etc.) — Peiman caught the systematic bias in the framing.
- Companion in auto-memory: `feedback_complexity_not_time.md`
