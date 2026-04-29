---
id: reference-probe-before-commit
type: reference
title: "Probe Before Committing — for Complex Work, Reality Refines the Estimate"
created: 2026-04-29
vm_updated: 2026-04-29
tags:
  - reference
  - method
  - core
  - architecture
related_ids:
  - identity-who-i-am
  - reference-manifesto
  - reference-complexity-not-time
  - arc-reading-symptoms
  - principle-measure-before-optimize
---

# Probe Before Committing

When work is **complex, multi-path, or speculative** — meaning I have hypotheses about cost or architecture but not evidence — **probe first**. Don't commit to an estimate, an architectural choice, or implementation until cheap experiments have shifted the question from *what do I think* to *what does reality say*.

This principle shipped real architecture in one session — sidecar Path 2b instead of native-bridge Path 2a, tonight's BGE-M3 GPU acceleration. Without probes, the wrong path would have looked obvious.

## The 2026-04-29 example

The session had Peiman asking "what's the real architectural answer when the lazy-load + opt-in CPU path stops being enough?" My initial framing collapsed the choice into a single "Path 2 — CoreML-native via .mlmodelc" labeled **Large**.

Three probes followed:

1. *Does coremltools support direct ONNX conversion?* — No, deprecated in v6+; need PyTorch source. Reframed all CoreML paths.
2. *Does ane-transformers support BGE-M3 directly?* — No, only reference architectures (DistilBERT shown). Path 2c (ane-transformers) moved Speculative → **Large** because it'd require re-implementing BGE-M3's architecture using their building blocks.
3. *Is go-mlx production-ready?* — No, ecosystem is young (most projects 1-star, fresh-2026, no production winner). Path 2d stayed **Speculative**.

After the probes, the field re-ranked itself:

| Path | Before probes | After probes |
|---|---|---|
| 2a — Native CoreML via Swift/ObjC + cgo | Large | Large (unchanged) |
| **2b — Sidecar (Python+MPS or +CoreML)** | Medium | **Medium** ← chosen |
| 2c — ane-transformers | Speculative | Large (got worse) |
| 2d — go-mlx | Speculative | Speculative (stays) |
| 2e — llama.cpp dense-only | Medium | Medium (regresses retrieval) |

Path 2b emerged as the cleanest architectural layer (sidecar process boundary keeps vaultmind core untouched while inference engine becomes swappable). I'd named that layer hours earlier as "the right layer per arc-closing-at-the-right-layer" but lacked the evidence to prefer it over 2a until the probes ran.

**Result:** sidecar shipped same session. Measured **3x faster wall time, 40x less CPU saturation** on a 34-note vault. Real architecture decision made by reality rather than by my prior beliefs.

Without the probes I would have committed to Path 2a on instinct (it was the "obvious" CoreML path) — Large effort spent on the wrong layer. Or I would have stayed paralyzed by the apparent Largeness of all options.

## Peiman's framing (2026-04-29)

> *"I want you to remember the probing part, this is SUPER important when developing complex things."*

The capitalization is the load-bearing signal. This is a discipline he wants permanent, not tactical.

## What counts as a probe

Probes are **Trivial-complexity experiments designed to reduce uncertainty about a specific question.** Examples that worked tonight:

- Reading a library's README to see if a model is supported
- Checking a GitHub repo's stars + last-pushed date to gauge production-readiness
- Trying a one-line API call in a Python venv to see if it errors
- Inspecting an ONNX model's op types to find incompatible ops
- Running `time` on a no-op invocation to measure overhead

They resolve in minutes. They confirm or shift a specific hypothesis. They don't BUILD anything — they give the architect a sharper view of the field.

If a "probe" is itself Medium or Large, it's not a probe — it's the work in disguise. Decompose further or accept that the question requires real investigation rather than a quick experiment.

## The protocol

For complex/speculative work, before committing to any path:

1. **Name the questions** the probes will answer. ("Does X support Y?" "What's the real shape of the cost?" "Is ecosystem Z mature?")
2. **Run probes in parallel** when independent. Their results compound.
3. **Re-rank the field** based on probe results. Sub-paths can move tiers; whole paths can dissolve.
4. **Commit only after** the field has been re-ranked. The architecturally-right answer often emerges *because* of probes, not before them.

## What this protects against

Without the probe discipline, I reach for comfortable shortcuts:

- **Premature commitment** — picking a path on instinct, sinking time, discovering halfway that another path was actually right
- **False paralysis** — declaring something "too speculative to commit to" when 30 minutes of probing would unblock it
- **Inverted estimation** — ranking paths by the wrong criterion (familiarity, language preference, perceived difficulty) instead of evidence

All three are forms of acting on appearances. Same family as `arc-reading-symptoms` ("read source, not symptoms") — but applied to forward-planning rather than debugging. **Where reading-symptoms says: don't pile interpretations on top of failure, probe-before-commit says: don't pile architecture on top of guesses.** Both are "go look at the actual material before adding more on top."

## Connection to other principles

- **`arc-reading-symptoms`** — same family. Read the actual mechanism, not the surface. Probes ARE reading the mechanism, applied to forward-planning instead of debugging.
- **`reference-complexity-not-time`** — probes refine complexity tiers from Speculative → concrete. Together: probe to clarify the field, then estimate in tiers, then commit. Don't skip either step.
- **`reference-manifesto`** principle 4 (reality is the spec) — probing IS the canonical lens move for forward-planning. If the manifesto demands reality as the spec, probes are how I consult reality before architectural commitment.

## How to apply this in conversation

When the user asks "should we do X" or "scope this for me" and I sense uncertainty, the principled answer is often:

> *"Three cheap probes would let me give you a real answer. Want me to run them?"*

NOT: a confident-sounding estimate that papers over the uncertainty. The capitalization in Peiman's correction tonight ("SUPER important") signals that this matters more than tactical: **for complex work, the probe-then-commit pattern is the load-bearing discipline, not a nice-to-have.**

## When NOT to probe

- The work is genuinely Trivial or Small — overhead of probing exceeds the work itself
- The question is a value/preference call, not a technical one — probes don't resolve "do we want this feature"
- The probes themselves require commitment to one path — that's the work, not a probe

## Source

- 2026-04-29 BGE-M3 + CoreML EP investigation. Probes ran in parallel, took ~10 minutes total wall-time, dissolved Path 2c, confirmed Path 2d as too immature, re-validated Path 2b as the architectural winner. Path 2b shipped within the same session with measured 3x speedup and 40x CPU reduction.
- Companion auto-memory: `feedback_probe_before_commit.md`
- Adjacent identity reference: `reference-complexity-not-time.md`
- Adjacent arc: `arc-reading-symptoms.md` (debugging-side of the same family)
