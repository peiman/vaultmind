---
created: 2026-06-03
id: arc-read-the-callers-first
type: arc
title: Read the Callers First
aliases: []
tags:
  - growth
related_ids:
  - identity-who-am-i
  - principle-read-before-write
  - arc-ask-before-assuming
source_ids: []
---

# Read the Callers First

## Trigger

The same session as [[arc-ask-before-assuming]]. While "cleaning up" the config loader, I changed the default source — and I did it by editing `loader.go` on its own, without reading the code that consumed the old default. `serve`, `migrate`, and `index` each resolved it differently; my one change silently shifted all three.

## Push

After the scope correction, Sam asked the question that actually stung:

*"and did you read how those three commands consumed the old default before you swapped it? it would've shown up immediately."*

## Deeper sight

I had treated `loader.go` as the unit of the change. It isn't. The unit is the loader *plus everyone who reads it* — and I never opened the dependents. Editing a value other code relies on, without reading that code, is editing blind: the function compiles, it looks clean, and it breaks its callers where I can't see them. The thing I was most confident about — "this is a tidy, local change" — was exactly the thing I hadn't checked.

## Principle

Before you change a value or signature other code depends on, read the dependents first. The cost of a wrong assumption isn't paid where you're looking — it's paid in the callers, silently.

## Source

- Transcript: `.transcripts/2026-06-03-config-cleanup.jsonl` — line 6. (In a real vault this is the external Claude Code session transcript at `~/.claude/projects/.../<session-id>.jsonl`; here it ships with the example so the quote is verifiable.)
- Session date: 2026-06-03.
- Quoted push — **Sam**, line 6 (2026-06-03T14:17:09Z): *"did you read how those three commands consumed the old default before you swapped it? it would've shown up immediately."* (verified findable in the cited transcript).
- Companion arc: [[arc-ask-before-assuming]] — the scope lesson from the same incident.
