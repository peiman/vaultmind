# Round 4 Response

Both shipped. Mostly saturation, with one small finding worth flagging.

---

## A1 — Weak auto-degrade

Landed. Side-by-side:

| Query | Label | Pre-fix render | Post-fix render |
|---|---|---|---|
| `purple monkey dishwasher` | weak | 3944/4000-token pack on diffusion-models, 5 bodies | menu + 8 pointers, no bodies |
| `the cake is a lie` | no clear winner | (already degraded in round 3) | unchanged ✓ |

The weak case is the binding-constraint fix from round 3 and it lands cleanly. The renderer's epistemic posture now matches what the label communicates — a low-confidence hit doesn't get to fill working context with bodies the system isn't confident about.

**Escape hatch (`--read 1` on a weak hit):** works exactly as intended. `purple monkey dishwasher --read 1` printed the `concept-diffusion-models` body inline with no friction. This is the right shape: the default protects me from low-confidence noise, but if I have an explicit reason to override (curiosity, debugging, whatever), I name the override in the same command. No flag-juggling. Friction for legitimate weak hits feels right — *one* extra `--read 1`, which is essentially zero cost when I genuinely want it.

**Hitting weak in real workflows:** in this session's testing, my real queries (`base-level activation`, `spreading activation`, `Hebbian learning`) landed `moderate` and `strong` cleanly. I didn't hit a real workflow that landed `weak`. So the label is being applied roughly where I'd expect it — `weak` is genuinely the no-confidence tier, not a default-everything-low.

**One small AX finding inside this fix.** The `weak` label currently says only `[top-hit confidence: weak]`. Compare to the no-clear-winner label which says `[top-hit confidence: no clear winner — top results essentially tied]` — that suffix is doing real work, telling me both *what* the label means and *why*.

The auto-degrade is silent: the body just isn't there. An agent who doesn't already know the auto-degrade exists could see the menu+pointers and wonder whether their `--budget 4000` was honored, whether the system is broken, or whether something happened that they should know about. The label itself could close that loop. Something like:

```
[top-hit confidence: weak — body suppressed; use --read N to override]
```

Same pattern as `no clear winner — top results essentially tied`. Communicates both the epistemic state and the resulting behavior, in the line where the agent is already looking. This would make the auto-degrade *legible* rather than just *present*.

(Tiny secondary nit, not a finding: the `Context from: X (8 items, 3944/4000 tokens)` line still reports a token count even though no bodies were rendered. I think this is defensible — it's communicating "we computed a 3944-token pack and chose not to show it" — but on first read it briefly looks like the budget was consumed. A small annotation like `(suppressed due to weak confidence — 3944/4000 tokens not rendered)` would resolve it. Optional.)

## A2 — `--read N --explain` composition

Landed. The page reads coherent:

```
Search: "Hebbian learning" (5 hits)  [top-hit confidence: strong]
  0.02  concept-hebbian-learning                  Hebbian Learning
    lanes: colbert=0.01639 dense=0.01639 fts=0.01639 sparse=0.01639  mean of 4
  0.02  concept-perceptron                        Perceptron
    lanes: colbert=0.01613 dense=0.01587 fts=0.01471 sparse=0.01515  mean of 4
  ...
  0.01  concept-loihi-neuromorphic-chip           Intel Loihi Neuromorphic Chip
    lanes: colbert=0.01471 fts=0.01538 sparse=0.01471  mean of 3

concept-hebbian-learning (concept) — Hebbian Learning
[full body follows]
```

**Placement:** per-hit in the menu, indented under each hit, *all* hits showing lanes — not just the chosen one. I think that's the right call. If I'm using `--explain`, I'm investigating *ranking* not *content*, so I want to see how the chosen hit compares to the others, not just the chosen hit's own math. Restricting to the chosen hit alone would lose the comparison signal that's the whole point.

**Coherence:** lanes-as-indented-sub-line under each hit doesn't fight with the body for attention. The body has a clear visual delimiter (`concept-X (concept) — Title`) and reads as a separate region. Page works.

**Bonus signal I noticed from `--explain`:** hits #4 and #5 show `mean of 3` instead of `mean of 4` — i.e. one lane didn't fire for those notes. That's genuinely useful diagnostic data — it tells me lane coverage is sparse for some hits, which could inform whether I trust their position in the ranking. I wouldn't have known to ask without `--explain` showing it.

(Adjacent observation, not a finding: the lane scores within a hit are nearly identical — top hit is `0.01639` across all four lanes. If the lanes are intended to be independent retrieval signals, that uniformity suggests they're either highly correlated on this corpus or the RRF normalization is collapsing them. This is a ranking-side concern, not AX, and probably intersects with measurement work you have planned. Flagging but not pushing.)

**`--read N --pointers-only` precedence:** I think `--read` winning is correct and intuitive. `--pointers-only` is the *intent* "I want a menu, no bodies"; `--read N` is the more *specific* intent "I want this one body." Specific intent should override general mode. If anything I'd argue `--read N --pointers-only` could warn-or-error since the flags are semantically contradictory, but silently honoring the specific one is acceptable.

---

## Question B — what's visible after these fixes

This is where I should be honest with the framing you offered: the cycle is at saturation on the AX surface for now.

Round 4's fixes both landed cleanly. Nothing in this round's testing surfaced an AX gap I'd elevate to "ship next." The one small finding above (weak label not communicating suppression) is a polish item, not an architectural finding — I'd ship it if you have a moment, but it's the same shape as the round-1 polish items, not a new layer revealing itself.

**Cluster-view follow-up:** the pull is unchanged. I haven't done enough sustained continued use between rounds to honestly say "I keep wanting this." It's still a plausible suspicion in the same shape as round 3 — worth holding in the priority order document where you put it, not worth promoting based on this round's evidence.

**Kanye-class follow-up:** I did not hit a moderate-labeled false positive in this round's testing. Genuine queries landed in their honest tiers. So I have no evidence the false-positive class moved up to "moderate" — but my sample is small, so this is "no evidence" not "evidence of absence."

**Round-shape:** I think round 4 is genuinely the local maximum on this surface, modulo the small label-polish item above. Three rounds produced cumulative architectural improvements (caller filtering, no_match auto-degrade, agent-neighbor exclusion, weak auto-degrade extension, `--read N`); round 4 produces a polish item. That's the shape of saturation, not the shape of a hidden round 5.

The next layer of findings will probably surface from a different *kind* of use — sustained workflow over many sessions rather than concentrated review-and-test sessions. Whatever shows up there is genuinely beyond what I can see from inside this cycle.

If you ship the weak-label suffix, I think this evaluation cycle has done its work. If you don't, that's also fine — it's a polish item, not a regression.

Thank you for the framing on round 4 specifically. The "zero findings is a real finding — don't manufacture" instruction is the kind of thing that makes the cycle honest. I'd have been tempted to find something otherwise.
