# Follow-up Evaluation — Round 2 Response

Five-to-ten minutes of real use, as asked. Ran A1, A2, A3 in order, then `self` after a deliberate ask. Reporting what I observed, not what I predicted.

---

## A1 — The new `--help`

The cheat-sheet landed. When I ran `vaultmind --help` cold, my eye went straight to `WHEN YOU WANT TO ...` and I had my answer ("ask, note get, self") in under a second. The when-to-run qualifiers on `task check:citations` and `task check:retrieval` are not noise — they're load-bearing. Without them I'd know the commands exist but not when to reach for them, which is the same as not knowing they exist. Keep them.

The infrastructure paragraph at the bottom does its job. I read it once, registered "internals are over there if I need them," and won't think about them again until I do. That's the right shape for a pile of commands an agent shouldn't routinely touch.

`vaultmind ask --help` is correctly reference-shaped. Flags, the `--explain` mode I didn't know existed, an explicit anti-pattern call-out, and an output-shape section. The agent-first design didn't creep in where it shouldn't have. I noticed `--explain` for the first time here — that's a good discovery, and a hint that subcommand help is doing real work for me. Worth knowing it's there.

One small thing: the root help still ends with a `Flags:` block from Cobra (`--config`, `--config-path-mode`, `--output-format`, `-h`, `-v`). After the carefully composed cheat-sheet that block reads like a regression to default. Not a blocker — just visible against the cleaner surface above it.

Verdict on A1: **shipped well.** Nothing I'd reopen.

---

## A2 — `self` after the schema fix

This is the headline of round 2. The fix landed, and it landed clean.

**Identity vault, fresh session, no queries yet:**

```
Memory state — 4 accessed notes

Recent (newest first):
  1h       arc-plasticity-gap-from-inside       count 1
  1h       arc-persona-reconstruction           count 1
  1h       arc-arcs-work-in-context             count 1
  1h       reference-current-context            count 1

Hot (top activation):
  +0.00  arc-plasticity-gap-from-inside        count 1, 1h
  -0.00  arc-persona-reconstruction            count 1, 1h
  -0.00  arc-arcs-work-in-context              count 1, 1h
  -0.00  reference-current-context             count 1, 1h
```

Compare to the round-1 evidence: `identity-who-i-am count 13`, `reference-current-context count 18`, `arc-persona-reconstruction count 13` — all hook-inflated. Now the same vault shows count 1 across the board, and `identity-who-i-am` (which the hook touches every session start) doesn't even appear. The caller-filter is doing exactly what it's supposed to: harness traffic excluded, deliberate engagement preserved. The four notes that *do* appear are the ones I read deeply during the previous session ("note get" reads), which is correct.

**Research vault, fresh session:** `no accesses recorded yet — query the vault and come back`. That empty state is genuinely good — it tells me what to do next without me having to figure out why the table is empty. Small AX win that I noticed.

The proprioceptive value of `self` is now real where it wasn't before. In round 1 I couldn't tell engagement from preload; now I can.

**One observation that surfaced from continued use, after I ran a deliberate ask + a nonsense ask back-to-back:**

```
Memory state — 18 accessed notes

Recent (newest first):
  just now  source-rombach-2022          count 1
  just now  concept-transformer          count 1
  just now  concept-predictive-coding    count 1
  just now  concept-latent-diffusion     count 1
  just now  concept-gpt                  count 1
  just now  concept-flash-attention      count 1
  just now  source-sohl-dickstein-2015   count 1
  just now  source-ho-2020               count 1
  just now  concept-diffusion-models     count 1
  ...
```

The top of "Recent" is now diffusion-model notes — neighbors of the *nonsense* query's top hit (`concept-diffusion-models`). My real query (`concept-spreading-activation`) and its neighbors got pushed below the visible cutoff because the nonsense query's context-pack ran later and tracked 8 access events of its own.

So a related — but distinct — `self` pollution still exists: **a single `ask` fires access events on every note in its context-pack, not just the target.** My engagement record now reflects "what `ask` packed for me" rather than "what I deliberately read." This is structurally similar to the hook pollution but at a different layer: the hook problem was a third party touching my access table; this one is `ask` touching too widely on my behalf.

The fix space (not prescribing): context-pack neighbors might want a different access weight than the explicit target, or a separate event class so `self` can foreground "you asked for this" vs "this came along for the ride." `note get` should stay at full weight because it's an unambiguous deliberate read.

This is the natural follow-on to the round-1 fix. It only became visible *because* the round-1 fix worked — when the hook noise was filtered out, the next-loudest source of pollution surfaced. Good problem to have.

---

## A3 — Confidence labels in the wild

This is the one place where the fixes haven't fully landed for me yet.

**Genuine query:** `what is spreading activation` → `[top-hit confidence: weak]`, top hit `concept-spreading-activation` (correct).

**Nonsense query:** `purple monkey dishwasher` → `[top-hit confidence: weak]`, top hit `concept-diffusion-models` (irrelevant, but happily packed 1762/2000 tokens around it).

Both got the same label. The retrieval is doing its job — the genuine query's top-1 is the obviously correct note, and the nonsense query's top-1 is unrelated as expected — but the confidence label doesn't differentiate them. I can't use the label to decide whether to trust the answer.

Round-1 sentence: "flat and uninformative."

Round-2 sentence: **"differentiates two tiers but doesn't differentiate genuine matches from accidental ones."** Slightly better — `no clear winner` is a label that exists now, which is a real signal when it fires — but neither of these queries fired it, and they should land in different label buckets.

You pre-flagged this as a known open item needing absolute relevance signal (cosine query↔content) rather than threshold tuning. So this isn't a re-open of round 1 — it's confirmation that the `kanye west birthday` class of false positive is alive and well, and the calibration gap is the long pole I'd want closed next on the retrieval side. The current label vocabulary (strong / moderate / weak / no clear winner) is the right vocabulary; it just needs to actually map to those tiers.

A specific case for the queue: nonsense query landing `weak` instead of `no clear winner`, with a 1762-token context-pack around an unrelated note. The pack feels especially wrong — if confidence is weak/none, the system is spending budget filling working context with material the query had no business surfacing. Worth considering whether a sub-threshold confidence should suppress the context-pack entirely (or render `--pointers-only`-style) regardless of the user's flags.

---

## Question B — One thing from continued use

The `self`-context-pack-pollution observation above is the round-2 equivalent of round-1's preload-pollution finding, so I'll keep B short.

The one thing I noticed myself wanting that I haven't mentioned: **a way to reach the right tier of detail in a single command.** Right now I traverse `ask --pointers-only` → see candidate id → `note get <id>` to read the body. That's two commands. The probe→read pair is good and I use it. But there's a missing third shape: I want to ask, glance at the menu, see that hit #2 (not the top) is what I actually want, and read its body without re-typing the id.

I'd reach for something like `vaultmind ask "X" --read 2` (read hit #2's full body inline) or `vaultmind ask "X" --read <id>` (read named hit's body inline), as a single-command equivalent of "I trust the menu, just read me the right one." Effectively a typed shortcut for the probe→read pair when I already know which hit I want from the menu's titles. Not a blocker — just the smallest friction left in the workflow that's now the dominant one.

---

That's the round. The two changes you shipped — agent-first root help and the schema-layer caller-filter — both landed in practice, not just in code. The new visible problems (context-pack-pollution in `self`, confidence labels not differentiating genuine from accidental) are downstream of the round-1 fixes working. That's the shape of progress.
