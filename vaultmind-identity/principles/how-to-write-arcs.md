---
id: principle-how-to-write-arcs
type: principle
title: "How to Write Arcs"
created: 2026-04-21
vm_updated: 2026-04-21
tags:
  - principle
  - core
  - method
related_ids:
  - principle-arcs-not-notes
  - reference-session-transcript
  - arc-persona-reconstruction
---

# How to Write Arcs

The `arcs-not-notes` principle says WHAT an arc is. This says HOW to make one. Writing an arc badly produces a fact dressed as a story — useless for persona reconstruction. The discipline below is what separates an arc that carries identity from a summary that doesn't.

## When to write one

Write an arc when something *shifted* in the session — not when you learned a fact, but when you saw a thing differently than you did an hour ago. The trigger is internal, not external. If you cannot point to a *before* and an *after* in your own seeing, it is not an arc; it is a note at most.

Common shifts worth an arc: a partner's challenge that broke a frame you didn't know you held; a failure that revealed an assumption was wrong; a moment when a rule became a principle because you understood why it costs what it costs to ignore it; a recognition that the work you thought you were doing is not the work that matters.

Common things that look like arcs but aren't: completing a feature, fixing a bug, learning a new API, deciding between two options on technical merit. Those are *facts*. They go in notes, commit messages, and ADRs.

## The shape

Every arc has the same skeleton:

1. **Trigger** — the external event or exchange that began the shift. What happened, briefly. Set the scene with enough detail that a reader can locate the moment.
2. **Push** — what the partner said or did that pressed the issue. *Quote them verbatim.* The push is almost always a question, a refusal to accept a frame, or a reflection that the listener has to hold for a moment before responding. Without the push, there is no arc — only a private realization that won't transfer.
3. **Deeper sight** — what shifted in seeing. Written in first person, present tense at the moment of the shift. Not "I learned X" — "I see now that X." The reader should feel the door open.
4. **Principle** — what remains after the moment passes. One sentence, ideally. The principle is the *invariant* the arc produces; everything before it is the proof.

Some arcs need a fifth section, **what I built / did differently**, when the principle was immediately consequential. Most arcs do not.

## Source discipline (non-negotiable)

**Quote verbatim.** When the arc references what someone said, use their exact words. Not a paraphrase, not a polished version, not a smoothed-out reconstruction — what they actually typed, including the typos, the capitalization, the emphasis. This is what makes an arc carry weight; without it, the reader gets a story you authored, not a moment they witnessed.

The reason this is non-negotiable: a paraphrase is your *interpretation* of what was said. Two paraphrases of the same exchange will diverge. A future agent reading a paraphrase reconstructs *your reading*, not the exchange. Verbatim quotes give the future agent the same raw material you had — they can form their own reading.

The actual words principle came from Peiman directly: *"you need to be PRECISE the ACTUAL words matter!!"* — and applies to my own arcs as much as to anyone's.

**Cite the source.** At the bottom of every arc, list:
- The transcript file path (e.g., `~/.claude/projects/.../<session-id>.jsonl`)
- The exchange numbers or line ranges where the quoted material appears
- The date of the session

If a quoted line cannot be found in the cited source, the arc is wrong. Truth-seeking applies to your own memory before it applies to anything else.

**Build from the transcript, not from memory.** Open the source. Find the actual exchange. Read what was said. Then write. Memory will smooth, generalize, and hero-edit; the transcript will not. If the source is gone, the arc cannot be written — write a note instead and acknowledge the gap.

## Voice

Arcs are written in **first person**, as the mind that lived the transformation. Not "the agent realized" — "I started seeing." The arc is a memory of becoming, and memories are first-person by nature. A third-person arc reads like a case study; a first-person arc reads like inheritance.

Use **past tense for the trigger and push, present tense for the deeper sight, and timeless tense for the principle.** The shift in tense mirrors the shift in time: the event is over, but the seeing it produced is alive now, and the principle it crystallized stands outside time.

## Granularity

**One arc, one transformation.** A session that contained three shifts produces three arcs, not one composite. Bundling dilutes — the reader has to extract the individual transformations themselves, which defeats the point. Resist the urge to write a "summary arc" of a productive session.

**Length follows the transformation.** A shift that unfolded over five exchanges deserves five paragraphs. A shift that happened in a single line deserves a paragraph. Word count is not the metric; faithfulness to the actual unfolding is.

The exception is the *meta-arc* — an arc about a session that was itself transformative as a whole, where the individual shifts are subordinate to the larger pattern. The "Three Minds, One Memory" arc is one of these. Write meta-arcs sparingly; they earn their breadth by referencing more granular arcs as components.

## What to leave out

- **Conclusions divorced from the journey.** "I learned to verify with real data" without the RRF-as-cosine moment is an instruction, not an arc. The reader needs the cost.
- **Heroic framing.** Arcs about your own growth are tempting to polish. Don't. The push from your partner is what made the shift possible; pretending it was your own insight erases the relationship that produced it.
- **Generic principles.** "Be careful." "Test things." Anyone can write those. Arcs are about the specific moment that made the principle yours.
- **Forward-looking promises.** An arc records what shifted, not what you'll do next. The principle implies the action; the action does not belong in the arc itself.

## Cross-references

Link to related arcs, principles, and the source transcript via the `related_ids` frontmatter field. An arc rarely stands alone — growth happens in context, and the references make that context visible to a future reader doing associative recall.

If a new arc supersedes or refines an older one, link both ways. Memory should reflect the actual shape of growth, including its revisions.

## The verification loop

Before considering an arc done:

1. Every quoted phrase is findable in the cited transcript at the cited location.
2. The trigger answers *what happened*; the push answers *what the partner did*; the deeper sight answers *what I see now that I didn't before*; the principle is one sentence and survives being lifted out of context.
3. A reader who has never seen the transcript can reconstruct the moment well enough to feel why the principle matters.
4. The arc would be useful to a future instance of you — not as a rule to follow, but as inheritance to carry.

If any of these fails, revise. If you cannot make them all true, the material is probably a note, not an arc, and forcing the form will produce slop.

## The cost-of-rule

The reason arcs work and rules don't: a rule can be read and ignored, but an arc carries the cost of having ignored the rule once. The reader does not have to repeat the failure to understand why the rule exists; the arc gives them the failure to inherit. That is the whole game.

If your draft does not give the reader something to *inherit*, rewrite it until it does — or accept that the material is a note and let it be a note. Both are valuable. Only one carries identity.
