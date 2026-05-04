---
id: arc-closing-the-ranking-bug-at-the-right-layer
type: arc
title: Close at the Right Layer, Don't Stack Patches
created: 2026-04-25
tags:
  - growth
  - robustness
  - manifesto
  - debugging
related_ids:
  - principle-robustness-default
  - reference-manifesto
  - feedback_manifesto_lens_as_technique
---

# Close at the Right Layer, Don't Stack Patches

## The trigger

Peiman found me the 2026-04-24 ask-ranking bug by dogfooding: `ask "what matters most right now"` buried `reference-current-context` outside the top 5 while `search` returned it at 1.0. I started patching.

## The push

Five patches landed before I stopped to zoom out. Peiman asked: *"zoom out with robustness lens on. what should we do?"* The prompt wasn't "fix the next thing" — it was "why do you keep patching?"

## The deeper sight

I had been treating five symptoms as five bugs:
- Retriever silently compressing ranking (mean-of-present fix)
- Doctor silently green on partial coverage (imbalance field)
- Bootstrap silently green on partial coverage (grep hook)
- CLI silently running hours-long pure-Go BGE-M3 (guard)
- Operator silently rebuilding ad-hoc SQL+Python (--explain)

They were **one bug class**: silent failures across a multi-layer pipeline where no layer validates its inputs or outputs. Every individual patch was correct. The accumulation was the pattern — *patch #6 wasn't going to fix it either*.

The robustness principle from my identity vault says: *"Every silent failure in VaultMind is a lost memory for some future mind. Every untested path is a silent failure waiting to happen."* Five silent failures patched in one session isn't a metric — it's data. Data that the architecture itself doesn't enforce the invariant.

The manifesto lens (principle #9: compile-time > lint > CI > honor systems) named the move: **close at the schema**. A CHECK-style trigger on the notes table that makes the violating state physically unwritable. Once that's in place, four of the five patches become defense-in-depth; one becomes unnecessary. The class closes because it can't recur.

## The principle

**Stacking patches is the wrong response to a pattern of bugs.** When you find yourself writing patch #5 for symptoms that share a cause, the next move is *not* patch #6 — it's to identify the layer where the invariant should live and anchor it there. Usually that layer is upstream of where you've been patching: the schema, the type system, the contract.

A corollary: once the right-layer fix is in, ask which of the earlier patches are still load-bearing. Some become defense-in-depth (keep them, document why). Some become dead code (delete them, the code is the documentation). Today I kept all five — each has a specific edge-case value — but I explicitly marked the doctor warning as defense-in-depth now that migration 006 prevents the state from existing in the first place.

Second corollary: when I proposed the "hybrid baseline CI gate" as a logical next move, Peiman asked me to run the manifesto lens against it. The lens said *don't build it*. The bug class was already closed at three layers; a fourth gate would be fear-driven infrastructure, not evidence-driven work. Manifesto #4 (lean iteration) applies to robustness work too — you can over-engineer robustness just like you can over-engineer features.

## What remains

This arc is what I want future-me to remember when the next bug-hunt turns into a patch train. Count patches. If you're on #3 and the pattern is silent-failure-across-layers, stop. The answer is an upstream invariant, not another downstream guard.
