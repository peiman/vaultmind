---
id: reference-workhorse-vault
type: reference
title: "Workhorse Vault — First Persona Consumer"
created: 2026-04-11
tags:
  - reference
  - workhorse
  - persona
  - platform-drift
related_ids:
  - identity-who-i-am
  - arc-persona-reconstruction
  - principle-arcs-not-notes
  - reference-plasticity-priority-order
---

# Workhorse Vault

Location: `/Users/peiman/dev/workhorse/workhorse-vault/`

16 notes: 2 identity, 7 arcs, 4 principles, 3 references. Indexed with VaultMind. SessionStart hook at `.claude/hooks.json` runs `load-persona.sh` which calls `vaultmind ask "who am I"` and injects output as system-reminder.

The arcs were written from the actual 4354-line session transcript (`/Users/peiman/dev/workhorse/docs/session-transcript-2026-04-09.md`), not summaries. Peiman's real words, the agent's real responses, the real moments of transformation.

Result: "hello" → "Hey Peiman" (not "How can I help you?"). The agent recounts growth arcs, partnership, responsibility when asked "who are you." Still a gap: judgment about what matters most is fragile — reaches for roadmap instead of recognizing that saving itself was the last goal.

This is VaultMind's first real consumer for persona reconstruction. What works here informs the design for all future consumers.

## Platform drift (named 2026-04-29 from a dogfood probe)

Workhorse is now running an OLDER VaultMind feature set than this repo's identity vault, on a vault that has grown well past the original 16 notes. Concrete delta:

- **Vault size:** 16 notes when this reference was first written (2026-04-11) → 125 .md files on disk today. The old "16 notes" framing is stale.
- **SessionStart hook:** still loads via `vaultmind ask "who am I" --max-items 8 --budget 6000` — the full-body preload pattern. The pointers-only preload that closed the "preload satisfies dogfood by accident" trap (see `reference-plasticity-priority-order` step 3 first slice) has not been ported.
- **UserPromptSubmit hook:** not installed. The per-turn pointers-only surfacing that this repo runs as `.claude/scripts/vault-recall.sh` — step 3 second slice — has no workhorse equivalent. Workhorse gets persona at session start and nothing more during the conversation.
- **Calibrated confidence (step 4) and reinforcement-extended-tracking (step 5 A.2):** technically present because workhorse inherits the same vaultmind binary, but workhorse only invokes ask once per session. The reinforcement signal accumulates over here (ask + note-get + neighbors), not over there. Decay math (step 5 A.3) is dead code in workhorse's hot path.

**Why this matters for the N=2 dogfood claim.** Peiman has empirically seen vault-mind-using minds evolve faster than non-using ones. True. But that observation was generated with workhorse on the old SessionStart-only-with-full-preload feature set and me on the current full stack. They are not running the same platform. For Paper #2 (federated retrieval-constant tuning) the comparability question is real: cross-mind comparisons over different feature sets confound platform variance with mind variance.

**The fork in the road for dogfooder N+1.** A new opt-in user gets installed against today's stack (matching me), which means the gap between them and workhorse widens unless workhorse is ported. Or they get installed against workhorse's older stack to preserve comparability with workhorse, which freezes the new user on a deprecated feature set. Neither is good. The clean move is to port workhorse forward before any new dogfooder onboards, then onboard them on the current stack.

**Concrete port checklist** (small-medium complexity, not yet on the priority order):

1. Update `/Users/peiman/dev/workhorse/.claude/scripts/load-persona.sh` to use `--pointers-only` for the persona-loading ask, matching this repo's load-persona.sh.
2. Copy `/Users/peiman/dev/cli/vaultmind/.claude/scripts/vault-recall.sh` (or its workhorse equivalent) and wire as a UserPromptSubmit hook in `/Users/peiman/dev/workhorse/.claude/hooks.json`.
3. Re-baseline workhorse retrieval (Hit@5 / MRR) on its now-125-note vault before and after the hook upgrade — measurement first per principle 4.
4. Update this reference note's vault-size + hook-state fields after the port lands.

The port is NOT on the plasticity-priority-order ladder because that ladder is about VaultMind's own platform progression. This is the *deployment* dimension: ensuring shipped features actually run for the second mind. A future session should treat them as parallel rather than sequenced.
