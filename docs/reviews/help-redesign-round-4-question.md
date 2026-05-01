# Round 4 Evaluation — Two Slices Shipped from Your Round-3 Response

Round 3's two findings shipped:

| Round-3 finding | Commit | What changed |
|---|---|---|
| Auto-degrade missed the kanye-class (weak tier still got full pack) | `823cba0` | Auto-degrade extends from `no_match` to also cover `weak` |
| `--read N --explain` silently dropped --explain | `823cba0` | `FormatAskReadWithOptions` plumbs explain through; menu hits show lane breakdown |

Plus your Question-B suspicion (`self --cluster` / `--thread`) recorded
in `reference-plasticity-priority-order` as a step-5+ horizon item —
deliberately not shipped, per your own probe-before-commit framing.

This round is **the smallest yet** — two specific checks, plus the
continued-use question. Both round-3 fixes are mechanical; if they
worked, you'll know in under five minutes.

---

## A1 — Weak auto-degrade

The binding-constraint fix you named. Run the kanye-class case from
round 3:

```bash
vaultmind ask "purple monkey dishwasher" --vault vaultmind-vault
```

Pre-fix (round 3): label was "weak" but the renderer happily delivered
3944/4000 tokens of context-pack around `concept-diffusion-models`.
Post-fix: weak → auto-degrade to pointers-only, no bodies, no
neighbors.

Plus the no_match case for control:

```bash
vaultmind ask "the cake is a lie" --vault vaultmind-vault
```

This already auto-degraded in round 3 — checking it stays right.

What I want to know:

- Does the default rendering now match the epistemic posture the
  confidence label conveys, or are there cases where you'd want the
  body of a weak hit by default?
- The escape hatch is `--read 1` (or `--read <id>`) when you actually
  want the body of a weak top hit. Run that:

  ```bash
  vaultmind ask "purple monkey dishwasher" --vault vaultmind-vault --read 1
  ```

  Does that feel like the right escape — explicit, single command,
  unambiguous about your intent? Or does the friction increase for
  *legitimate* weak hits feel too high?
- Is "weak" still surfacing as the label often, or did the
  recalibration leave most real queries at "moderate" or above? (The
  label is the same; only the rendering changed. So this is about
  whether you hit weak in real workflows now.)

## A2 — `--read N --explain` composition

Run:

```bash
vaultmind ask "Hebbian learning" --vault vaultmind-vault --read 1 --explain
```

Pre-fix: --explain silently dropped, body printed alone.
Post-fix: each menu hit shows its per-lane RRF math (`fts=`, `dense=`,
`sparse=`, `colbert=` with the mean-of-N annotation), then the chosen
note's body below.

What I want to know:

- Is the lane breakdown placed where you'd want it (above the body,
  per-hit in the menu)? Or would you have wanted the breakdown only
  on the chosen hit, with the rest of the menu staying terse?
- Does the resulting page feel coherent, or does the lane math fight
  with the body for attention?
- Any other --read + flag combinations you'd reach for that I should
  test? (`--read N --pointers-only` is structurally weird — pointers-
  only means "don't show bodies" but --read explicitly asks for one.
  Currently `--read` wins. Worth your read on whether that precedence
  is intuitive or counterintuitive.)

## Question B — what's visible after these fixes

Same shape as rounds 1–3. The pattern has held three times: each
round's fixes have made the next round's findings visible. Round 4 is
the test of whether it keeps holding when the round's fixes are
smaller and more targeted than the previous rounds'.

Specifically:

- **Cluster-view follow-up:** in round 3 you flagged `self --cluster`
  as a suspicion you didn't want to ship without stronger pull. Three
  more sessions of normal use later (or however much real use you've
  had since round 3): is the pull stronger? Weaker? Did continued use
  surface a *different* `self`-shape you'd want first?
- **Kanye-class follow-up:** does the auto-degrade extension feel
  sufficient, or did it just push the false-positive class somewhere
  else (e.g. now the FTS lexical hit lands at "moderate" because the
  threshold cutoffs aren't quite right)? If you're hitting moderate-
  labeled false positives now, that's the next ranking-side gap.
- **Round-shape observation:** rounds 1–3 each produced 1-3 concrete
  findings you'd ship if you could. If round 4 produces zero, that's
  evidence the cycle has reached a local maximum on the AX surface.
  Zero findings is a real finding — please flag if that's where you
  land. Don't manufacture a finding for the sake of completing the
  form.

---

## How to respond

`docs/reviews/help-redesign-round-4-response.md`. Free-form prose,
partial answers fine, "I have nothing to add" is also a real signal.

This is round 4 of a four-round cycle that's clearly produced real
architectural improvements. If the cycle has reached a saturation
point, that's the most useful thing you can say. If round 4 surfaces
something new, that's the next loop. Either is good.

Thank you again.
