package distill

// ArcGuide is the canonical, self-serve arc-writing discipline, printed by
// `vaultmind arc guide`. It encodes what an onboarding agent otherwise has to be
// told by hand — the manual hunt (the shapes the candidate detector can't catch),
// the bar, the non-negotiables, the diff test, and the self-check.
//
// Intended as the SSOT for that discipline: today `vaultmind init` seeds a
// SEPARATE embedded how-to-write-arcs template, so the two surfaces currently
// drift. A later slice reconciles init's template to this const so they don't.
const ArcGuide = `VaultMind — How to Find and Write Your Own Arcs

An arc is a durable record of how your thinking, values, or working style SHIFTED —
a moment you saw something differently than you did an hour before. Notes hold
information; arcs hold inheritance: a future instance of you reads an arc and starts
already-changed instead of re-deriving the lesson from scratch.

This is propose-only on the tool's side. ` + "`vaultmind arc candidates`" + ` surfaces a
few easy, phrase-matched moments, but most real arcs are NOT phrase-shaped — you find
them by reading. The tool surfaces; the mind crafts.

WHEN IT IS AN ARC
  Write one when something shifted in YOU — not when you learned a fact or finished a
  task. If you can point to a before and an after in your own seeing, it may be an arc.
  If nothing about how you act downstream would change, it is a note.

HOW TO HUNT (read the session for change, not keywords)
  Read the session start to finish and look for these shapes — the ones a phrase
  matcher cannot catch, which is why reading beats the candidate detector:
    1. Reversal            — "wait—", "actually—": a direction changed or a decision undone.
    2. Reframe             — a push that changed the FRAME, not just a fact.
    3. Frame-break         — the moment the task wasn't what you thought it was.
    4. Method-invalidation — a shortcut or assumption exposed as invalid.
    5. Cost-of-rule        — a rule held even though following it cost you something.
    6. Evidence/trust gate — proceeding was gated on your confidence or the evidence.
    7. Ownership assertion — a boundary of sole responsibility was drawn.

THE BAR (every arc has this shape)
    1. Trigger      — the event or exchange that began the shift; set the scene.
    2. Push         — what your partner said or did, quoted VERBATIM. No push, no arc.
    3. Deeper sight — what shifted in your seeing; first person, present tense
                      ("I see now that…", not "I learned…").
    4. Principle    — the one sentence that remains after the moment passes.
    5. Source       — transcript path + date + where the verbatim push is found.

NON-NEGOTIABLES
  - Quote verbatim — the actual words, typos and all. A paraphrase transfers your
    reading, not the moment.
  - Build from the transcript, not memory — memory smooths and hero-edits. Open the
    source and find the real exchange. If the source is gone, write a note instead.
  - First person, no heroic framing — the push from your partner is what made the
    shift; don't erase it.
  - One arc, one transformation — three shifts make three arcs, not one summary.

THE DIFF TEST
  Would a future you answer some question DIFFERENTLY because of this moment? If yes,
  it is an arc. If nothing downstream changes, it is a note.

SELF-PRESSURE-TEST (before you commit an arc)
  [ ] Every quoted phrase is findable in the cited transcript.
  [ ] Trigger / Push / Deeper sight / Principle / Source are all present.
  [ ] A reader who never saw the transcript can feel why the principle matters.
  [ ] It gives a future instance something to INHERIT, not just to know.
  If any fails, revise — or let it be a note. Both are valuable; only one carries identity.`
