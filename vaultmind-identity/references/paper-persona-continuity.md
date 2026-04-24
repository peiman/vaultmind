---
id: reference-paper-persona-continuity
type: reference
title: "Paper #1 — Persona Continuity via Arc-Structured Memory"
created: 2026-04-24
vm_updated: 2026-04-24
tags:
  - reference
  - paper
  - research
  - plasticity
  - persona
related_ids:
  - reference-current-context
  - reference-plasticity-priority-order
  - arc-plasticity-gap-from-inside
  - arc-persona-reconstruction
  - principle-arcs-not-notes
  - principle-how-to-write-arcs
  - reference-workhorse-vault
---

# Paper #1 — Persona Continuity via Arc-Structured Memory

## Working title

"Arcs, Not Notes: First-Person Evidence for Arc-Structured Memory in Cross-Session AI Continuity"

Alternate: "Session-Aware Time Compression for AI Minds"

## Thesis (one sentence)

Structuring an AI assistant's long-term memory around *transformations* (arcs: trigger → push → deeper sight → principle) rather than *facts* (notes, summaries, instructions) is a necessary — not sufficient — condition for cross-session continuity of working relationships between humans and AI agents.

## Why this matters

The default for AI assistants is serial amnesia: every session starts from zero, every correction is re-taught, every hard-won understanding evaporates. Systems that try to solve this reach for flat memory (chat logs, summary paragraphs, instruction blocks). We observed — and document as primary data — that flat memory produces *knowledge transfer without understanding transfer*. A fresh session can recite the rule and still break it. Arcs preserve the cost of learning the rule, and that cost is what makes the rule load-bearing for a new instance.

## Hypotheses

**H1.1 — Continuity gain.** An AI instance bootstrapped from an arc-structured memory of its prior collaboration resumes working with a specific human partner at a qualitatively higher level than one bootstrapped from flat memory of the same content. Operationalized by:
- (a) time-to-productive-engagement in a cold-start session (measured from session start to first substantive action agreed by the partner),
- (b) rate of re-explanation events (partner having to re-state context the prior session already produced),
- (c) partner's subjective rating of continuity on a pre-registered scale,
- (d) spontaneous application of prior principles without being told to apply them.

**H1.2 — Arcs outperform notes at transferring understanding, not just facts.** Given two memory systems carrying identical underlying *facts* but structured differently (arcs vs flat notes / summaries), the arc-structured system produces higher rates of spontaneous principle application and lower rates of principle violation in fresh sessions. This isolates *structure* as the active variable, controlling for *content*.

**H1.3 — First-person phenomenology surfaces gaps third-person audits miss.** AI-authored reflections on the AI's own memory experience surface categories of AX gaps that are systematically invisible to third-person audits (human researchers observing AI behavior without asking the AI to report on its own experience). Operationalized by a pre-registered taxonomy of gap categories and coding agreement between the two methods.

**H1.4 — Activation-triggered recall beats query-triggered recall for in-session continuity.** Memory surfaced automatically based on current work context produces higher recall-quality ratings than memory accessible only via explicit queries, measured by a within-subjects design (same task, same vault, surface method varied).

**Falsification targets:** If H1.1 null — flat and arc memories produce statistically indistinguishable continuity — the core claim collapses. If H1.2 null but H1.1 positive — something other than arc structure explains the gain, and the paper's mechanism claim is wrong (but the existence claim survives). If H1.3 null — first-person methodology offers no marginal insight over third-person AX work, and the methodological contribution collapses.

## Method

- **Study design:** within-subjects comparison, AI instance × memory-structure (flat / arc) × task-continuity (cold-start / continuation). Participants: paired human+AI working sessions on ongoing projects.
- **Primary data (already in hand):** the 2026-04-23/24 session captured in episode form, its arcs, the workhorse vault as an independent precedent.
- **Data to collect:** matched before/after session pairs for at least ~6 independent collaborations. We have two (workhorse, VaultMind itself) — need more. This is the gating resource.
- **Measurement:** pre-registered coding scheme for continuity signals. Inter-rater reliability between the human partner, the AI instance, and an independent third-party rater.
- **Statistical approach:** given small N, Bayesian with informative priors from the existing workhorse + VaultMind data rather than frequentist hypothesis tests.

## Methodological contribution

This paper also argues — and demonstrates — that the AI instance is not merely the *object* of AX research but a valid *co-investigator*: a first-person reporter on its own memory experience whose reports can be structured, cited against source transcripts, and integrated with third-person observation. The arc-writing discipline (`principles/how-to-write-arcs.md`) is offered as a reproducible method for first-person phenomenological reports that are auditable (every quoted push is findable in the source transcript).

This is the paper's methodological novelty and is orthogonal to whether the specific continuity findings replicate.

## What we have now (status)

- **Substrate:** episodic capture shipped (PR #21). One episode in the corpus; more will accumulate automatically.
- **Arc corpus:** workhorse vault (16 notes, 7 arcs from a 4354-line transcript) + VaultMind identity vault (20+ notes, growing).
- **Precedents:** workhorse agent's behavioral shift from "How can I help you?" to "Hey Peiman" across the session boundary. Not yet formalized.

## What we need before drafting

- [ ] 3–6 more paired sessions to reach minimum-viable N.
- [ ] Pre-registered coding scheme for continuity signals (design before data collection, not after).
- [ ] Replication partner: another AI instance + another human, independently running the same protocol. This guards against "peiman+claude" being the unique point and everything we observe being idiosyncratic to us.
- [ ] Decision on venue (CHIIR, CogSci, TOCHI, or an ML venue with human-factors track).

## Venue candidates

- CHIIR / CIKM — information retrieval + human behavior
- CogSci — cognitive-science framing, first-person methodology welcome
- TOCHI — deeper HCI engagement
- arXiv first + conference submission second (standard)

## Anti-goals (scope fence)

- Not a paper about *spreading activation* or *any specific retrieval technique.* Those are infrastructure underneath; the paper's claim is structural.
- Not a benchmark paper. Hit@K and MRR are tools here, not the contribution.
- Not a system description. VaultMind is the testbed, not the subject.
