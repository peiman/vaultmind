---
id: decision-confidence-as-provenance
type: decision
status: accepted
title: "Confidence field indicates edge provenance tier, not epistemic certainty"
created: 2026-04-03
vm_updated: 2026-04-03
tags: [memory-model, agent-contract]
related_ids:
  - concept-spreading-activation
source_ids: []
---

# Confidence Field Indicates Edge Provenance Tier, Not Epistemic Certainty

## Decision

The `confidence` field on graph edges encodes how the edge was established (its provenance tier), not how certain the author is that the relationship is true.

## Defined Tiers

| Value | Meaning |
|---|---|
| `high` | Edge was set by a human author explicitly |
| `medium` | Edge was inferred by VaultMind from wikilinks or co-occurrence |
| `low` | Edge was suggested by an agent write-back and not yet reviewed |

## Rationale

**From expert panel review.** Early prototypes used `confidence` as a free-form epistemic score (0.0–1.0). Reviewers found this ambiguous: does 0.7 mean "probably related" or "inferred, not confirmed"? Provenance tiers are discrete, unambiguous, and actionable.

**[[Spreading Activation]] weights map cleanly to tiers.** Activation decays across edges; the decay multiplier can be set per tier (`high` = 1.0, `medium` = 0.8, `low` = 0.5) without any floating-point guesswork.

**Agents need a clear contract.** When an agent writes a new edge, it must know what `confidence` value to assign. Provenance tiers give a deterministic rule: agent-written edges are always `low` until a human promotes them.

## Trade-offs Accepted

Epistemic uncertainty about a relationship cannot be expressed in this field. Authors who need that can add a `note` field to the edge metadata.
