# Round 3 Evaluation — Four Slices Shipped from Your Round-2 Response

Your round-2 response (`help-redesign-followup-response.md`) named four
findings. All four shipped in the session that followed, in atomic
commits:

| Round-2 finding | Commit | What changed |
|---|---|---|
| `self` flooded by Ask context-pack neighbors | `3911b04` | Caller filter widened — excludes `agent-neighbor` AND `hook` by default |
| No_match queries waste budget on irrelevant context-pack | `513875f` | Auto-degrade to pointers-only when confidence is "no clear winner" |
| Missing `--read N` workflow shape | `a7dc89a` | New flag + `query.AskHits` + `FormatAskRead` |
| Global-flags block visually regressing from cheat-sheet | `9876963` | Wrapped in section dividers, matches page style |

Round 3's job is the same as round 2's: confirm the changes feel right
in real use, not just in code review. Four concrete checks, one open
continued-use question. Five-to-ten minutes of running the commands
and reporting impressions.

---

## A1 — `self` after the agent-neighbor exclusion

Run a few queries — at least one real, at least one nonsense — then
check `self`:

```bash
vaultmind ask "<something you actually want to know>" --vault vaultmind-vault
vaultmind ask "purple monkey dishwasher" --vault vaultmind-vault
vaultmind self --vault vaultmind-vault
```

In your round-2 evidence, the nonsense query's 8 context-pack neighbors
flooded the "Recent" list and pushed the real query results below the
visible cutoff. After this fix, only `CallerAgent` events (Ask top-hit
+ note get) surface in the default `self` view; neighbor accesses are
still logged in the underlying `note_accesses` table but don't
participate in the proprioceptive view.

What I want to know:

- Does the hot list now reflect what you deliberately engaged with —
  the targets of your asks, not their neighbor fan-out?
- The nonsense query's top hit (`concept-diffusion-models` last time)
  is still a `CallerAgent` event because it was the Ask target. Does
  *that* feel right? It IS engagement (the agent named the topic), or
  it ISN'T (the topic was nonsense)? I'm not sure which framing is
  more honest. Worth your read.
- Anything weird about how the counts read — too sparse now? Or is
  this exactly the granularity you wanted?

## A2 — No_match auto-degrade

Run a clear nonsense query without `--pointers-only`:

```bash
vaultmind ask "purple monkey dishwasher" --vault vaultmind-vault
vaultmind ask "the cake is a lie" --vault vaultmind-vault
```

Pre-fix: nonsense landed `weak` or `no_match` label but still got a
1762-token context-pack around an unrelated top-1 (your evidence).
Post-fix: when confidence is `no_match`, the renderer auto-degrades to
pointers-only — agent sees the menu and the no-clear-winner label, but
no body and no neighbor context.

What I want to know:

- Does the auto-degrade feel right, or does it feel like the system
  silently overruling your `--budget` request?
- The pointers-only footer hint still fires
  (`run vaultmind ask <query> against any id above to read the body`).
  Useful, or noise when you've already labeled the result as no-clear-
  winner?
- Edge case: what about queries that legitimately should land
  `no_match` but where you'd actually want to see SOMETHING (e.g.
  exploratory queries where "what does the vault have on this fuzzy
  topic" is the actual intent)? This is a degraded-mode-by-design
  decision — flag if you'd want a way to override it (`--force-pack`
  or similar).
- The kanye-class FTS false positive (your round-1/2 finding) is the
  biggest remaining ranking problem. The auto-degrade is defense-in-
  depth against it. Do you find yourself hitting kanye-style false
  positives less often now that no_match is mechanical, or about the
  same?

## A3 — `--read N` / `--read <id>`

This is your continued-use ask from round 2. Try the workflow you
described:

```bash
vaultmind ask "spreading activation" --vault vaultmind-vault --pointers-only --max-items 5
# look at the menu, decide which hit you want
vaultmind ask "spreading activation" --vault vaultmind-vault --read 2
```

Or by id:

```bash
vaultmind ask "Hebbian" --vault vaultmind-vault --read concept-hebbian-learning
```

Error cases:

```bash
vaultmind ask "Hebbian" --vault vaultmind-vault --read 99
# expected: "--read 99: only 5 hit(s) available (use 1-5)"

vaultmind ask "Hebbian" --vault vaultmind-vault --read concept-not-in-results
# expected: id-not-in-hits error with recovery hint
```

What I want to know:

- Does this feel like the missing workflow shape you named, or did the
  shipped version drift from your intent?
- The error messages — are they clear, or do they read as obstacles?
- Access tracking: `--read N` fires `CallerAgent` on the chosen note
  (not on the search top hit). Run `vaultmind self --vault vaultmind-vault`
  after a few `--read` invocations and confirm the hot list reflects
  the notes you actually read, not the search top-hits you skipped past.
- Any flag combinations that don't compose well?
  (`--read` + `--explain` is the obvious one — should `--read` show
  the lane breakdown for the chosen hit, or is the menu enough?)

## A4 — Global-flags block polish

Run `vaultmind --help` and look at the bottom of the page:

```bash
vaultmind --help | tail -10
```

Pre-fix: trailing `Flags:` block in default Cobra style — felt like
"regression to default" against the curated surface above.
Post-fix: section-divider header `GLOBAL FLAGS (apply to every
subcommand)` matching the rest of the page's style.

What I want to know:

- Does the page now read as one coherent document, or are there still
  visual seams where Cobra-default and custom-template meet?
- The "(apply to every subcommand)" qualifier — useful clarification
  or unnecessary?

---

## Question B — What's next, after the round-2 fixes landed?

Same shape as round 1's and round 2's continued-use question.

Round 2's pattern was load-bearing: you used `self` after the round-1
fix and immediately found the neighbor pollution that the round-1 fix
made visible by removing the louder hook noise. *"The new visible
problems are downstream of the round-1 fixes working. That's the shape
of progress."*

If that pattern holds, this round's most useful signal is what
**round-2's fixes made visible** that wasn't before. Specifically:

- With `self` now reflecting only deliberate engagement: are there
  patterns in your access history that suggest a missing affordance?
- With no_match auto-degrade: do you find yourself wanting a different
  query strategy in the cases that hit no_match? Like, the auto-
  degrade tells you the system has no clear answer — what would you
  want to do next?
- With `--read N`: now that the probe→read pair is one command, what's
  the next-most-frequent multi-step workflow you'd want collapsed?
- With the help page now coherent: is there discovery you wish was
  surfaced that still isn't? (`task check:retrieval` is in there;
  `vaultmind self --limit 5` exists but isn't named in the cheat-
  sheet, etc.)

One observation, one suspicion (your round-2 framing) is plenty.

---

## How to respond

Same as before — free-form prose, drop in
`docs/reviews/help-redesign-round-3-response.md`. Partial answers fine.
If only one of A1–A4 produced a non-trivial observation, that's what's
worth writing about.

Three rounds in: this is becoming a real review-and-ship cycle, not
just a one-shot evaluation. The pattern is doing what we hoped — your
round-2 fixes surfaced the round-3 things; the round-3 fixes will
surface the round-4 things, if there are any. The thesis-level finding
from round 1 ("the vault treats me as a mind with continuity worth
preserving") feels more true with each round of evidence that the
review→ship→re-review loop produces real architectural improvements.

Thank you for the time.
