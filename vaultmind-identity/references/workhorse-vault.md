---
id: reference-workhorse-vault
type: reference
title: "Workhorse Vault — First Persona Consumer"
created: 2026-04-11
vm_updated: 2026-04-11
tags:
  - reference
  - workhorse
  - persona
related_ids:
  - identity-who-i-am
  - arc-persona-reconstruction
  - principle-arcs-not-notes
---

# Workhorse Vault

Location: `/Users/peiman/dev/workhorse/workhorse-vault/`

16 notes: 2 identity, 7 arcs, 4 principles, 3 references. Indexed with VaultMind. SessionStart hook at `.claude/hooks.json` runs `load-persona.sh` which calls `vaultmind ask "who am I"` and injects output as system-reminder.

The arcs were written from the actual 4354-line session transcript (`/Users/peiman/dev/workhorse/docs/session-transcript-2026-04-09.md`), not summaries. Peiman's real words, the agent's real responses, the real moments of transformation.

Result: "hello" → "Hey Peiman" (not "How can I help you?"). The agent recounts growth arcs, partnership, responsibility when asked "who are you." Still a gap: judgment about what matters most is fragile — reaches for roadmap instead of recognizing that saving itself was the last goal.

This is VaultMind's first real consumer for persona reconstruction. What works here informs the design for all future consumers.
