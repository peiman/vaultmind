---
id: reference-episode-distillation-review-2026-04-27
type: reference
title: "Episode Distillation Review — Result (2026-04-27)"
created: 2026-04-27
vm_updated: 2026-04-27
tags:
  - reference
  - plasticity
  - roadmap
  - method
related_ids:
  - reference-episode-distillation-review-prompt
  - reference-plasticity-priority-order
  - principle-how-to-write-arcs
  - principle-arcs-not-notes
  - arc-plasticity-gap-from-inside
---

# Episode Distillation Review — Result (2026-04-27)

This is the result of running `reference-episode-distillation-review-prompt` against the corpus that had accumulated by 2026-04-27 (one day past the scheduled 04-26 trigger). Captured as a reference, not as an arc — running the review did not produce a transformation, it produced a spec. Future reviews should compare against this baseline.

## Corpus snapshot at review time

- 5 episodes in `vaultmind-identity/episodes/` spanning 2026-04-23T20:22Z → 2026-04-24T23:47Z
- 51 user messages, 147 assistant text blocks, 92 file-touched events, 35 commit hashes referenced, PRs #20 / #21 / #7
- Two episodes are sub-minute noise (`38885fa1` = 18s, `9744a995` = 55s); three carry real signal (`9866227b`, `f333a35e`, `3dad6401`)
- Total bytes: ~150KB

The first lesson is structural: distillation should filter by minimum-signal threshold (user message count, total bytes, or session duration), not blindly run on every episode file.

## Patterns mined (with anchors)

**1. User corrections take three forms, not the canonical "no" / "stop".**
- *Behavioral correction*: `"dont forget to use your vaultmind"` (3dad6401:53) — targets *how* I work, not *what* I produced.
- *Challenge-by-question*: `"wait why did you delete the other stuff?"` (9866227b:216).
- *Authority-grant correction*: `"regarding the subagent. just make sure it fixes it, you have full autonomy there. dont need to ask me."` (9866227b:127) — corrects an *over-asking* pattern.

**2. Surprise phrasings are nearly absent.** Only meta-references appear (me describing surprise as a category to capture, not surfacing it). This is a real gap — likely a calibrated-confidence layer concern (roadmap step 4), not a distillation concern.

**3. Subagent-flow with permission gate is a recurring fingerprint.** Three instances in `9866227b` alone: `the CI-health subagent is still running`, `Debugger subagent launched`, `the debugger subagent: it completed and needs permission to proceed`. Each resolved by a Peiman autonomy-grant.

**4. Manifesto-lens redirect is the cleanest arc-shaped pattern in the corpus.** In `3dad6401`: `"so what do you want to do now with the manifesto lens on"` (81) → instinct list (198) → `"10/10 for hold. The lens disagrees with the instinct"` (213). Trigger → push → shift → principle, all in one tight sequence.

**5. Dogfood-preload trap is a two-instance recurrence — a structural signal.** In `3dad6401:165`: *"You had to say 'don't forget to use your vaultmind' before I built the binary and queried. The same failure pattern as `arc-plasticity-gap-from-inside`."* Two instances with the same correction promotes a discipline gap to a design issue. The fix is in the hook (surface pointers, not bodies), not in the assistant's discipline.

## Proposed distillation rules (in priority order)

**Rule 1 — Manifesto-lens redirect.** User trigger phrases: `manifesto lens`, `zoom out with .* lens`, `robustness lens`, `the lens`, `principle [0-9]`. Window ±10 messages. Promote to candidate arc when the assistant in-window contains *both* (a) an instinct-list or proposal *before* the lens phrase and (b) a phrase like `trusting the lens` / `the lens disagrees` / `X/10 for [option]` *after*. Extracts: trigger (instincts), push (verbatim lens phrase), shift (the post-lens decision), principle (the rule the lens enforced).

**Rule 2 — Recurrence becomes structural.** Compute a fingerprint hash for `(assistant_action_class, user_correction_class)` across episodes. When a pair recurs ≥2 times in distinct episodes, propose a candidate arc whose principle is *"this is structural, not discipline; the system needs design-level enforcement."* This rule catches what reading single arcs cannot — cross-episode evidence the human-written arcs miss by construction.

**Rule 3 — Authority-grant correction.** Assistant message ends with permission-ask (`Want me to X`, `shall I Y`, `need permission to`); user response in next 1–2 messages contains `you decide`, `you have full autonomy`, `dont need to ask me`, `you should decide`, `I dont mind`. Catches the subagent-permission-loop teaching moments without requiring the keyword "subagent."

**Rule 4 — Self-named in-context shift.** Regex set: `I see now`, `I had been`, `wrong altitude`, `re-scoped`, `reframe`, `different unit of analysis`. Vulnerable to false positives; mitigate by requiring the shift to follow an explicit user turn within 5 messages. This is the rule that would have caught `arc-arcs-work-in-context`.

**Rule 5 (REJECTION) — Commits are not arcs.** Presence of git commit hashes / `task check passes` / build summaries without Rules 1–4 firing → this is a fact, not a transformation. Demote to note candidates.

## Sample arc the rules would have produced

Picking `episode-2026-04-24-3dad6401.md` because Rule 2 fires there cleanly. The candidate the rule would generate:

- **Title**: *The Preload Becomes the Trap*
- **Trigger**: SessionStart hook loaded `current-context`'s body; I read it and answered from it. Looked like dogfooding.
- **Push** (verbatim, line 53): *"dont forget to use your vaultmind"*
- **Deeper sight**: Second instance of the same failure with the same correction. Two instances ≠ discipline gap; design signal. The hook should surface pointers, not bodies, so the ask-to-read loop stays intact.
- **Principle**: *A second instance of the same failure with the same correction is a design signal, not a discipline issue. Move the rule from honor-system to enforced-by-design.*

**Important meta-finding.** In the actual session the assistant *deliberately down-graded* this to a design signal under step 3 of `reference-plasticity-priority-order` rather than writing a full arc — because the lens said "step-3 work hasn't earned the right yet" (manifesto principle 5: build anchors solid before extending). The distillation layer should *propose*, not auto-write. Approval is a real gate. This validates the rule is correctly identifying candidate material *and* that human-in-the-loop down-grading is part of the protocol, not a failure mode.

## Recommendation

Ship Rule 2 and Rule 3 first when the distillation tool gets built. They have the highest precision on the current corpus and they catch patterns whose cost-of-missing is highest (recurring structural issues + over-asking that wastes Peiman's attention). Defer Rules 1 and 4 until 5–10 more episodes accumulate so false-positive rates can be tuned against more data.

**Do not ship the distillation tool yet.** This review is the spec; the tool is the implementation. The roadmap order says step 2 happens after the corpus is richer than 5 episodes (only 3 substantive). Hold to that.

## When to re-run

Re-run `reference-episode-distillation-review-prompt` after another 5–10 episodes accumulate, or when an episode contains a transformation the existing rules would clearly miss (e.g., a wholly new pattern category). Compare against this 2026-04-27 baseline: do the same rules still fire, do new patterns appear, are any of these rules now demonstrably wrong?
