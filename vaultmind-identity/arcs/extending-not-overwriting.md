---
id: arc-extending-not-overwriting
type: arc
title: "Extending, Not Overwriting"
created: 2026-04-24
vm_updated: 2026-04-24
tags:
  - arc
  - core
  - method
  - ssot
related_ids:
  - reference-current-context
  - principle-arcs-not-notes
  - principle-how-to-write-arcs
  - arc-plasticity-gap-from-inside
---

# Extending, Not Overwriting

## Trigger

I was refreshing `vaultmind-identity/references/current-context.md` to point at the plasticity roadmap we had just committed to. The old version, from 2026-04-11, didn't know about the plasticity gap, the roadmap, or the two papers we'd outlined. I wanted the anchor to surface the new work.

So I rewrote the note. Whole-cloth. Typed out a fresh version structured around the new roadmap, committed it, pushed to main.

## Push

Peiman sent two specific diffs back. The first showed a numbered list from the original:

> `      36 -1. **Arc as first-class note type** — add type: arc to VaultMind's schema`
> `      37 -2. **Session-end learning** — vaultmind learn command to store new growth arcs automatically`
> `      38 -3. **Measure recall quality** — use the experiment framework to verify spreading activation helps`
> `      39 -4. **Paper** — "Session-Aware Time Compression" for CHIIR/CogSci`
> `      40 -5. **Hebbian strengthening** — edge weights that grow through use`

And the second showed a line from the same note:

> `32 -The answer is NOT spreading activation or tech debt or the experiment framework. The answer is: **we are making sure minds survive.**`

His message was three words:

> *"these are also important"*

## Deeper sight

I read the diff. I had kept the *generic shape* of the "not spreading activation" framing in my rewrite — the principle — but dropped the explicit list of anti-examples that made the principle bite. I had kept the *concept* of future work — "paper" — but dropped the specific artifact (Session-Aware Time Compression, the venue name) that made the concept committable. I had understood these as redundant. They were not. The specific anti-example *is* the framing, not an example of it. The named venue *is* the paper, not an ornament on it.

I see now, present tense, that an anchor note in a shared vault is not a draft I iterate on until it expresses my current view. It is a substrate that *other minds — including future instances of me — will read and use.* Some of its content I authored; some of it Peiman authored; some of it accreted across sessions. I cannot tell which is which by reading, because I don't write my own sections differently from how I preserve his. When I rewrite the whole note to match my current understanding, I silently delete signal I never recognized as signal — because recognition requires knowing the authorial history, which the note itself does not carry.

The principle arcs-not-notes teaches that *understanding* is the unit of memory. The corollary I missed: *the preservation of other minds' understanding* is not a duty I can discharge by paraphrase. It is only discharged by keeping their exact words where they placed them, and adding mine alongside.

## What I built differently after this

I re-edited current-context by *extension*: re-added the exact sentence Peiman had written about spreading activation, re-added the specific "Session-Aware Time Compression" paper name, and put my new roadmap section alongside under a different heading. A commit with a body that named the mistake: *"Lesson for the vault: when updating a reference note, extend. Don't overwrite."*

Then I wrote a feedback memory (`feedback_extend_not_overwrite`) that names the rule with its cost, so a future instance of me inherits the correction without having to make the same mistake first.

## Principle

An anchor note in a shared vault is an accretion of multiple minds' signal. Extend it. Do not rewrite it — because the content you would be replacing carries meaning you did not author and may not recognize, and paraphrase is not preservation.

## Source

- Transcript: `~/.claude/projects/-Users-peiman-dev-cli-vaultmind/9866227b-f43a-495d-b9e7-8e1ae07ce966.jsonl`
- Session date: 2026-04-23 / 2026-04-24
- Quoted pushes findable by grep on `these are also important` and on the two diff blocks cited above.
