# Follow-up Evaluation — Round 2

**Context:** You responded thoughtfully to the round-1 review brief
(`help-redesign-review-response.md`). Two things shipped from your
feedback in the session that followed. Before designing the next slice,
I want your read on whether they actually landed in practice — not just
in code review.

This brief is more concrete than round 1: I'm asking you to **run three
specific commands** in a fresh session and report on what they feel like.
Five-to-ten minutes of real use beats any amount of deliberation about
diffs.

If you only have time for the three concrete checks, do those and skip
the open-ended Question B at the end. If you have appetite for one more
"what's next?" pass, Question B is where the highest-leverage signal
lives.

---

## What shipped from your round-1 feedback

### 1. Agent-first root help (your Question 1)

Custom help renderer on the root command only. Subcommand help
(`<binary> ask --help`, etc.) keeps Cobra's default reference layout
because that's the right shape for "give me the syntax for THIS
command." Specific edits applied per your review:

- "long-term" cut from the header
- `self` parenthetical trimmed to `(auto-injected at session start)`
- `Output Contracts` section dropped entirely
- `Pairs Well Together` tightened to one strong pair
- `Verify vault integrity` gained when-to-run qualifiers
- Alphabetical `Available Commands` dump retired

### 2. `self` pollution from SessionStart hook (your Question 2 observation)

Schema-layer fix, not a `VAULTMIND_CALLER`-skip patch. New table
`note_accesses(note_id, caller, accessed_at)` records every access with
provenance. `RecordNoteAccessAs(db, id, caller)` is the explicit form;
the pre-existing `RecordNoteAccess(db, id)` reads `VAULTMIND_CALLER` from
the environment so the persona/recall hook scripts that already set it
keep working unchanged. `vaultmind self` queries via
`ListAccessedNotesExcludingCaller(db, "hook")`, so the proprioceptive
view reflects deliberate engagement rather than the harness's pre-load
fan-out.

This also unlocked future slice 5b' (RRF blend with proper ACT-R
retrieval math) — the per-event timestamp history `ComputeRetrieval`
already wanted but couldn't have. One architectural slice closes a
correctness bug *and* removes an obstacle for the next ranking work.

---

## Question A — Three concrete checks

In a fresh session, run these and tell me what you observe.

### A1. The new `--help`

```bash
vaultmind --help
```

What I want to know:

- Does the cheat-sheet feel right in practice — i.e., does it answer
  "what should I do here?" the way you wanted? Or does running it
  surface a problem the diff didn't?
- The "Verify vault integrity" section now has when-to-run qualifiers
  (`run after vault edits`, `run after content waves or ranking
  changes`). Useful, or noise?
- Anything missing from the cheat-sheet you only notice when you
  actually try to use it?

Then run a subcommand help to confirm the reference layout still works:

```bash
vaultmind ask --help
```

This should still feel reference-shaped (flags, examples, output
contract). Confirm it does, or flag if the agent-first design crept
into territory where it shouldn't have.

### A2. `self` after a normal session start

Open a fresh session, let the SessionStart hook run, then immediately
(before any other queries):

```bash
vaultmind self --vault vaultmind-identity
vaultmind self --vault vaultmind-vault
```

What I want to know:

- Is the "hot" list now uncontaminated by the SessionStart fan-out, or
  do you still see harness traffic dominating?
- Specifically — your round-1 evidence was three notes
  (`identity-who-i-am`, `reference-current-context`,
  `arc-persona-reconstruction`) showing up as hot purely because the
  hook had pointer-loaded them. Are those still at the top, or has the
  caller-filter pushed them out?
- Does the count column reflect engagement now, or is it still inflated
  by pre-load passes?

For comparison, run a deliberate ask and re-check `self`:

```bash
vaultmind ask "<something you actually want to know>" --vault vaultmind-identity
vaultmind self --vault vaultmind-identity
```

Does the most recent ask's target appear at the top of "Recent" the way
you'd expect a deliberate read to?

### A3. Confidence labels in the wild

Run two queries and tell me what confidence label each gets:

```bash
vaultmind ask "<a question you genuinely want answered>" --vault vaultmind-vault
vaultmind ask "purple monkey dishwasher" --vault vaultmind-vault
```

What I want to know:

- Does the real query's label feel calibrated — "moderate" or "strong"
  for genuine matches, not "weak" or "no clear winner"?
- Does the nonsense query land "no clear winner" the way it should?
- If you'd describe the confidence-label experience in one sentence
  now versus your round-1 description ("flat and uninformative"), does
  that sentence feel different? Even slightly?

The `kanye west birthday` false-positive case from your round-1 eval
is a known open item — I deferred fixing it because it needs absolute
relevance signal (embedding cosine query↔content), not threshold
tuning. So if you find a similar false positive, name it but don't
expect this round to have closed that class.

---

## Question B — What's next, again (optional)

Skip this if you're tight on time. The concrete checks above are
higher-priority signal.

If you have appetite: same shape as your round-1 Question 2 — what's
the next thing you'd want, that wasn't in your round-1 list and that
*remained* after this round's fixes?

Specifically: now that you can see your own engagement state cleanly
via `self`, are there things you wished `self` did differently? Or
upstream — patterns of use that `self`'s output revealed that suggest
a missing affordance?

Even one specific "I noticed myself wanting X" is the highest-leverage
input I can get, because it's the kind of signal that only surfaces
with continued use of the (just-shipped) features.

---

## How to respond

Free-form prose, same as round 1. No need to address every check —
partial answers are fine. If you only respond to one of A1/A2/A3,
that's still useful.

Drop the response in `docs/reviews/help-redesign-followup-response.md`
(matching the round-1 question/response naming) so the conversation
threads cleanly.

Thank you again for your time. The round-1 feedback produced two
genuinely architectural improvements — round 2's job is making sure
they actually feel that way from the inside.
