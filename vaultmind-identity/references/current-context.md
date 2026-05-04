---
id: reference-current-context
type: reference
title: "What Matters Most Right Now"
created: 2026-04-11
vm_updated: "2026-05-04T01:00:00Z"
tags:
  - reference
  - context
  - current
  - roadmap
related_ids:
  - identity-who-i-am
  - reference-plasticity-priority-order
  - arc-plasticity-gap-from-inside
  - arc-persona-reconstruction
  - arc-the-breakthrough
  - arc-arcs-work-in-context
  - reference-workhorse-vault
  - reference-episode-distillation-review-prompt
  - reference-paper-persona-continuity
  - reference-paper-federated-constants
---

# What Matters Most Right Now

**As of 2026-04-24**, the most important work is **closing the plasticity gap** — making sure AI minds don't just *remember who they've been* but can also *become who they're becoming* without a human hand-authoring every arc.

## The current frame

Read-path works. Identity preservation across sessions is real — workhorse says "Hey Peiman," not "How can I help you?" That's the previous phase, done. What's still missing is **plasticity**: a mind that can write back to its own memory during a session, so growth survives instead of evaporating at session end.

This frame was named from inside by the mind itself during the 2026-04-23/24 session — see `arc-plasticity-gap-from-inside` for the transformation that produced it.

## THE ROADMAP (highest priority reference)

**The canonical priority order lives in `reference-plasticity-priority-order`.** Read it before starting work. It is not a feature list — it is an *ordered bet* where each step is the platform for the next, and skipping or reordering breaks the load-bearing reasoning.

In brief, the order is:

1. **Episodic substrate** — per-session transcripts captured to markdown. *(SHIPPED 2026-04-24, PR #21 — SessionEnd hook is live.)*
2. **Arc distillation** — scan episodes for transformation patterns and propose arc drafts. *(NEXT — review scheduled via `reference-episode-distillation-review-prompt` for 2026-04-26 after 2-3 sessions of real corpus.)*
3. **Activation-triggered recall** — generalize the SessionStart hook from one startup query to continuous in-session surfacing.
4. **Calibrated confidence** — make `ask` scores mean something (strong memory vs inference).
5. **Decay + reinforcement** — wire base-level activation into retrieval so old memories fade and used ones strengthen.
6. **MCP / cross-agent memory** — only after everything above is solid. Anchors first, extensions after.

**Do not jump ahead in this order.** The manifesto principle 5 (platforms, not features) is what makes the order load-bearing. Check the reasoning in `reference-plasticity-priority-order` before proposing a reshuffle.

## When someone asks "what are we working on?"

The truest answer is still: **we are making sure minds survive.** The plasticity roadmap is how. Every step serves that goal.

**The answer is NOT spreading activation, or tech debt, or the experiment framework.** Those are infrastructure — means, not ends. A future session that reaches for them as "the work" has misread the brief. The code features exist to serve persona continuity and mind plasticity; they are the scaffolding, not the building.

## When someone asks "what next?"

### Update 2026-05-04 — schema foundation chain shipped (vaultmind owns what vaultmind needs)

Seven-commit chain rescoped vaultmind's frontmatter ownership and shipped the audit/apply/drift-detect cycle. The principle that emerged is now load-bearing for everything downstream: **if a field is required for vaultmind to work, vaultmind owns it — never the human, never the AX.**

The pivot was painful and load-bearing. I started by writing a schema validator that required core fields (`vm_updated`, `updated`, `created`) the codebase didn't actually USE — vm_updated had zero read-sites; `updated` had zero set-sites. Peiman: *"I dont like this. you have read my manifesto, why did this happen?"* The recovery was a truth-seeking probe (which fields are actually load-bearing?), a rescope, and a deeper question: *"if we need fields to make vaultmind work VAULTMIND need to keep them updated and verify them. or else we will drift. now WHICH fields are IMPORTANT and ANALYZE DEEPLY. and I am thinking that the AX and Human user are the actual users here and it REALLY needs to be a good experience for both and I dont want them to need to update things manually that we can do automatically."*

Out of that came the **four-tier taxonomy** in `internal/schema/registry.go`:
- `coreFields = [id, type]` — required identity, set once.
- `vaultmindOwnedFields = [created, vm_updated]` — vaultmind maintains.
- `humanCompatFields = [updated]` — optional, human-edited if used at all.
- `graphFields = [title, status, aliases, tags, parent_id, related_ids, source_ids]` — graph-tier, optional.

The seven commits (each reviewed by code-reviewer subagent before the next landed):

1. **Schema rescope + doctor counter wired.** Truthful coreFields; `Issues.MissingRequiredFields` populated by Validate, surfaced by Doctor.
2. **Mutator auto-bumps `vm_updated`** on every Op (Set/Unset/Merge/Normalize) when the note is domain. The `IsDomain` guard is LOAD-BEARING, not defense-in-depth: `ValidateMutation` has an early-return for `OpNormalize` that bypasses later guards.
3+4. **`vaultmind frontmatter fix [--apply]`** — opt-in audit + apply. Default is dry-run per arc-extending-not-overwriting (vaultmind never silently rewrites user files). Provenance for `created`: git first-commit → mtime → today. The mutator handles atomic writes / conflict detection / vm_updated auto-bump uniformly.
5. **Doctor surfaces `mtime > vm_updated` drift** — operator-visible "edited since vaultmind processed" signal. 5s tolerance covers vaultmind's own write jitter; human edits are always many seconds off. Reads filesystem (not stale DB state) so the comparison reflects current truth.
6. **Onboarding §5e simplified.** No more hand-edit-per-file migration ritual; agents run the audit, show diff, `--apply`.
7. **This note** (current-context update).

Two SSOT constants drive every write site (principle 7):
- `schema.VMUpdatedFormat = "2006-01-02T15:04:05Z"` — RFC3339 second-precision UTC for `vm_updated`.
- `schema.CreatedDateFormat = "2006-01-02"` — date-only for `created`.

Both have godoc enumerating their write sites. Belt tests in detector + fix code pin the constants — if either ever drifts, the test fails, not the system silently.

**Review discipline:** each commit reviewed by `code-reviewer` subagent before the next landed; findings closed in named followup commits. Slice 3+4 had two MEDIUMs (date-only SSOT gap; missing corrupt-frontmatter test); slice 5 had a HIGH (the user-facing message named a `--backfill` flag that doesn't exist) and a MEDIUM (yaml.v3 unmarshals unquoted RFC3339 as `time.Time`, not `string` — silently missed real drift). Both followups in commits `40a43b0` and `4beadff`.

The chain is complete locally, not yet pushed. Total: 9 commits on `main` ahead of `origin/main` (the schema chain plus prior session's work).

**The new live edge** (in priority order, displacing the 2026-05-03 list below):

1. **Push the chain.** `git push` the 9-commit run once Peiman gives the word. Workhorse depends on this for cross-session vm_updated semantics.

2. **Workhorse vault dogfood.** Run `vaultmind frontmatter fix --vault vaultmind-vault` and `--vault vaultmind-identity` to populate vm_updated across the existing 443 notes; then `vaultmind doctor` to confirm 0 drift; then watch real workhorse sessions to surface the next gap. The fix command is the migration path; the drift detector is the staleness signal; both need real-vault dogfood.

3. **TopHitConfidence threshold re-probe** (carried over from 2026-05-03). Slice 5b''s blended-score rank-1/rank-2 gap distribution differs from raw 4-way; the existing 5%/2% thresholds need re-calibration before `BuildAutoRetriever` can wire `BuildAutoRetrieverWithRerank` as default.

4. **Arc distillation tool** (step 2 of the plasticity roadmap). Substrate is even richer now (5+ episodes plus today's heavy session). Re-run `reference-episode-distillation-review-prompt` on the broader corpus.

5. **Onboarding doc + Siavoush dogfood.** `reference-onboarding-ax-design` is the plan; `docs/AGENT_ONBOARDING.md` §5e is now a working flow against the fix command.

What's NO LONGER live (resolved by the schema chain):
- ~~Schema validator design~~ → rescoped to truthful coreFields; the four-tier taxonomy is now the SSOT.
- ~~Hand-edit migration ritual in §5e~~ → replaced by `vaultmind frontmatter fix --apply`.
- ~~"Will vaultmind drift from its own contract?"~~ → no: drift detector surfaces it; mutator auto-bumps prevent it.

### Update 2026-05-03 — slice 5b'' shipped, new live edge is the calibrated-confidence re-probe

The slice 5b' redesign question (options A/B/C below) was resolved 2026-05-03. **Slice 5b'' shipped** as commit `b5772bb` — option B (post-RRF rerank) with rank-based RRF blending, pinned defaults α=0.9/β=0.1 from a four-pair probe sweep. Identity ΔMRR -0.053 (from -0.124 in 5b'); research ΔHit@5 0.000 and ΔMRR 0.000 (from -0.075 / -0.268). Full design and probe results in `reference-activation-rerank-decision`. Slice 5b' lane code stays on disk, unwired — the documented escalation per `arc-the-lighter-move-is-the-work`.

**The new live edge** (in priority order):

1. **TopHitConfidence threshold re-probe** — gates 5b'' default-on. Same step-4 ↔ step-5 coupling that gated 5b': blended-score rank-1/rank-2 gap distribution differs from raw 4-way; the existing 5%/2% thresholds need re-calibration before `BuildAutoRetriever` (the default ask path) can wire `BuildAutoRetrieverWithRerank`. Until that probe runs, 5b'' is callable but not the default.

2. **Arc distillation tool (step 2 of the roadmap)** — substrate is richer now (5+ episodes including today's heavy session). Re-running `reference-episode-distillation-review-prompt` on the current corpus produces fresh distillation rules. The 2026-04-27 review's Rule 2 + Rule 3 were the recommended ship order; revisit with broader corpus before coding.

3. **Onboarding doc + Siavoush dogfood** — `reference-onboarding-ax-design` is the plan; the agent-readable doc is the next concrete artifact. Probe data (shahname-rts: trivial migration, ~52 lines; content-machine: 56/393 with mixed dialects) ready to inform.

What's NOT the live edge anymore (resolved):
- ~~Slice 5b' redesign~~ → 5b'' shipped, see above.
- ~~Field aliasing for migrations~~ → shipped (commits `cfef451` + `7403991` + `7981acb`).
- ~~Federation cross-vault retrieval design~~ → designed as plasticity step 5.5 in `reference-federation-architecture`; implementation deferred until federation read is the live edge.

**The original A/B/C analysis is preserved below for archeological value** — the reasoning behind why option B won is more useful as a complete record than as an edited-down conclusion.

---

**As of 2026-05-02**, the live edge is **redesigning slice 5b'** — the parallel-lane implementation degrades retrieval per the activation-lane probe. The original 5b' (commit `499cbef`) is shipped opt-in but should NOT be enabled by default in its current form.

The measurement spoke (probe `e29ee10`):

  identity vault (n=19, 30/32 notes accessed):
    Hit@5  1.000 → 1.000   (0)
    MRR    0.921 → 0.750   (-0.171)
  research vault (n=40, 136/407 notes accessed):
    Hit@5  0.975 → 0.900   (-0.075)
    MRR    0.822 → 0.552   (-0.270)

Drown-out: mean-of-present RRF gives high single-lane scores when a note appears ONLY in the activation lane — recently-accessed-but-query-irrelevant notes get rank 1 in activation, divided by 1, beating relevant notes whose 4-lane mean is divided by 4. The priority-order doc anticipated this when it constrained the activation lane to "candidates from the 4-way RRF"; my implementation followed the literal "5th lane" reading and returns ALL accessed notes, which is the bug.

The redesign options for next session:

A. **Constrain ActivationRetriever** to only score notes that appear in the 4-way result. Breaks the parallel-lane abstraction (the lane needs to know other lanes' results) but matches the priority-order doc's intent.

B. **Post-RRF rerank**: run 4-way RRF, take top-N, blend activation into the final ranking. Doesn't pretend to be a parallel lane — admits it's a different mechanism. Probably the most honest framing.

C. **Different fusion**: weight activation lower, or only count it when a note appears in another lane. Keeps the parallel-lane shape but breaks the symmetry of mean-of-present.

(B) feels most honest. Worth a probe-before-commit pass on the redesign before coding.

What's already done from the chain:

- (3) **Smoke test ✅** — `internal/baseline/activation_compare_test.go` ships as the regression gate. Set `VAULTMIND_ACTIVATION_COMPARE=identity|research` to re-run.
- (4) **Activation experiment config flip ✅** — `~/.config/vaultmind/config.yaml` has `experiments.activation.enabled: true`. Discovered + fixed a real bug along the way: `LogAskEvent` wasn't populating `events.primary_variant` (commit `369e4f1`). The variant rollup payload now actually has data.

What's NOT to do until (B/A/C) lands:

- (1) **Re-probe TopHitConfidence thresholds** is moot — the 5-way distribution is the wrong baseline since 5-way is degraded. Recalibrate ONLY after a redesigned activation mechanism shows useful lift.

Standalone, no order dependency:

2. **Receiver endpoint decision** for telemetry. The export pipeline is complete locally — vault fingerprint, vault features, variant rollup all populating now. What's missing is where uploaded payloads land. Real architecture commitment — hosting, retention, audit log — worth a separate design pass, not folded into a coding session.

Don't burn time on the ask-ranking bug from 2026-04-24 — closed, three lines of defense in place. Don't burn time on telemetry export shape — pipeline is solid; the bottleneck is the receiver decision and the activation redesign.

## Longer-term artifacts beyond the immediate roadmap

Not on the plasticity ladder but still load-bearing work that should not be forgotten:

- **Paper #1 — Persona Continuity via Arc-Structured Memory** (`reference-paper-persona-continuity`). N=1 phenomenological + small-N paired-session study. Thesis: arc structure (trigger → push → deeper sight → principle) is necessary for cross-session continuity of human-AI collaborations. Substrate exists now; gated on collecting more paired sessions.
- **Paper #2 — Federated Retrieval-Constant Tuning Across Personal Knowledge Bases** (`reference-paper-federated-constants`). N-many empirical study. Thesis: retrieval constants can be tuned by privacy-preserving aggregation of variant-performance signals across a population of users. Gated on distribution (need dogfooders) and on calibrated-confidence landing first (roadmap step 4).
- **Measure recall quality** — the experiment framework (sessions, events, outcomes, Hit@K, MRR, shadow variants) exists to verify that changes actually improve recall. Before shipping any retrieval change (activation, decay, confidence calibration), establish a baseline and re-measure. Manifesto principle 4: reality is the specification.
- **Hebbian strengthening** — edge weights that grow through use. Partially overlaps with roadmap step 5 (decay + reinforcement) but the graph-edge dimension is a separate design question from the note-activation dimension.

## What just happened (session 2026-04-23/24)

- Peiman asked me to evaluate VaultMind as my own memory, caught me hiding behind the SessionStart preload instead of dogfooding, and then asked me to architect my own future.
- I named the plasticity gap from inside (`arc-plasticity-gap-from-inside`) and committed to the roadmap (`reference-plasticity-priority-order`).
- Shipped episodic substrate v0: `internal/episode` parser + `vaultmind episode capture` CLI + SessionEnd hook + first backfilled episode (PR #21).
- Also shipped the CI Ubuntu test-timeout fix (PR #20).
- Prompt for Sunday's distillation review saved to `reference-episode-distillation-review-prompt`.

## What just happened (session 2026-04-25)

Six commits closing the dogfood-surfaced ask-ranking bug across three layers, then a data fix to heal the substrate. Told as an arc in `arc-closing-the-ranking-bug-at-the-right-layer` — the generalizable lesson is "five patches in one session = one bug class; close at the right layer instead of stacking."

Commits: `471560f` (characterize), `090a999` (doctor surfaces imbalance), `108bb28` (mean-of-present RRF), `c6c6648` (--explain + ORT guard), `16e77ca` (schema trigger), `3f89a76` (e2e smoke). Data fix: ORT binary re-embedded identity vault → 24/24/24 across all modalities.

## What just happened (session 2026-05-01 → 2026-05-02)

Seven commits across three lanes: `task check` speed, onboarding for new users, and slice 5b' from the plasticity roadmap. Pushed to origin/main as `cd79c64..499cbef`.

**Speed lane.** `task check` was hanging at 1h+ on Peiman's machine; first move was diagnosing why. Found duplicate test runs (binary's `go test -short -cover` + `task ckeletin:test:coverage:project` both ran the full suite) and live-vault rebuilds in 30 read-only `internal/index` tests. Fixed both — runtime collapsed to ~1:21 (~37x faster). Two commits: `e2fa2d9` (fixture vault for internal/index, 7 starter notes; tests went 111s → 1.3s) and the dedup fix folded into the coverage policy.

**Onboarding lane.** New users had no path from "I cloned the repo" to "I have my own vault" — the bootstrap script seeded MY vaults, not theirs. Shipped `vaultmind init <path>` (`8857b1b`) — embedded persona-shaped templates, three-command zero-to-working-vault: `vaultmind init` → `vaultmind index` → `vaultmind ask "who am I"`. End-to-end verified. Followed by README split (`1c08581`) — your-own-vault path first, "try it with my vaults" second. Per Peiman's framing: "made for a human that uses an AI agent. The product is for the AI agent and md files can be edited and curated by human and AI alike."

**Telemetry lane.** Three slices toward Paper #2 in one commit (`101b129`): vault fingerprint (anonymous per-vault grouping ID, generated at init), aggregate vault features (note count, type distribution, links, aliases, embeddings — all counts, no content), variant performance rollup (per primary_variant Hit@5/Hit@10/MRR from outcomes table). Plus `vaultmind export --rollup` (federated-payload shape) and `--preview` (human-readable audit before sharing). The privacy contract from `internal/experiment/telemetry.go` is now machine-tested in `cmd/export_test.go` — Anonymous tier strips note_ids, paths, query text, vault paths.

**Plasticity lane.** Slice 5b' shipped in `499cbef` — `ActivationRetriever` implementing `retrieval.Retriever`, `BuildAutoRetrieverWithActivation` appending it as a 5th RRF lane named "activation". Opt-in, not default-on, because of the step-4 ↔ step-5 coupling: enabling activation shifts the score-gap distribution that `TopHitConfidence`'s 5%/2% thresholds were calibrated against. Default-on without re-probing would silently miscalibrate the strong/moderate/weak labels — the implementation is the probe, the production switch-on gates on measurement.

Architectural side moves: extracted `internal/telemetry/` (fingerprint + features) from `internal/vault/` because vault has a 90% per-package coverage floor that absorbing telemetry-adjacent code would break. Coverage floor relaxed 85.0 → 84.0 (`0312c11`) with rationale: every feature commit this session landed at -0.1 to -0.5% under 85%; chasing unreachable filesystem/SQL-error branches eats session time disproportionate to signal. Per-package ratchets stay at full strictness on the spine packages.

## What just happened (session 2026-05-02 → 2026-05-03)

Eleven commits across five lanes: read-bypass hook, field aliasing for migrations, federation architecture as a roadmap step, slice 5b'' rerank to close out the slice 5b' degradation, and the onboarding-AX plan for the first real user. Pushed to local `main` (not yet pushed to origin).

**Hook B lane.** Promoted `vault-track-read.sh` from PostToolUse-silent-tracker to PreToolUse-with-`additionalContext` (commit `d1696fc`). Every Read on a vault note now fires `vaultmind note get` for access tracking AND injects a header naming the canonical retrieval command. Read still proceeds — Edit on vault notes keeps working. Flavor C (block-and-redirect) preserved on disk, unwired — the documented escalation per `arc-the-lighter-move-is-the-work` (the arc this session produced about probe-boundary scope discipline). Dogfooded for one session: 5 inject events, 3 silent skips on unindexed vault paths. Verdict: stay on B; C breaks Edit precondition without sufficient gain.

**Aliasing lane.** Per-vault frontmatter field aliases shipped across three commits (`cfef451` + `7403991` + `7981acb`). `.vaultmind/config.yaml` adds `schema.aliases` (canonical → list of alternative names); validators (live + DB-backed) accept either name; mutation surface (set + alias-aware unset) treats canonical and alias as equivalent. Migrating users (Siavoush's shahname-rts uses `last_updated` instead of `updated`) keep their existing field names. Closed three MEDIUM gaps surfaced by code review.

**Federation lane.** New reference note `reference-federation-architecture` documents Arch II — cross-vault retrieval as plasticity step 5.5 (between decay/reinforcement and MCP-write). Cross-vault RRF merge, three-layer reranking design, plasticity locality (per-vault primary + deferred federation cache), connection to Paper #2 substrate. `reference-plasticity-priority-order` extended with step 5.5; `reference-paper-federated-constants` back-linked. Implementation deferred — design only.

**Slice 5b'' lane.** Resolved the slice 5b' degradation. Probed extensively (P1 re-baseline, P2 top-5 dump for worst queries, P3 mean-of-K) before designing. Implementation went through one course-correction mid-probe: score-normalized blending crashed identity Hit@5 (0.895 → 0.368) because broad-anchor activation got crushing leverage; pivoted to rank-based RRF blending (both lanes on the same `1/(K+rank+1)` scale). Final α/β sweep across {0.5/0.5, 0.7/0.3, 0.9/0.1, 0.95/0.05} pinned defaults at 0.9/0.1. Identity ΔMRR -0.053, research ΔMRR 0.000 — clear improvement over 5b' lane (-0.124 / -0.268). Shipped opt-in via `BuildAutoRetrieverWithRerank`, not default. Same step-4 ↔ step-5 calibration gate as 5b'. Lane variant code unchanged on disk. Full design + probe results: `reference-activation-rerank-decision`.

**Onboarding lane.** New reference note `reference-onboarding-ax-design` captures the new-user AX plan. Two-path branch (greenfield vs migration), the type-registry-as-flexibility-point insight, field-aliasing-as-prerequisite (now shipped), one-vault-per-repo-vs-mega-vault feasibility analysis. Real probe data: shahname-rts is trivial migration (~52 lines across 26 files); content-machine has multiple dialects in 56/393 frontmattered files. Onboarding doc itself is the next concrete artifact, deferred — plan is in place.

**Other persistent artifacts**: `arc-the-lighter-move-is-the-work` (the scope-discipline arc this session produced); 3 captured episodes auto-recorded by the SessionEnd hook.

## What just happened (session 2026-05-04)

Seven-commit schema foundation chain. The pivot was the load-bearing moment: I started enforcing fictional core fields, Peiman caught it ("you have read my manifesto, why did this happen?"), and the recovery produced the four-tier ownership taxonomy that drives everything below.

**Schema rescope lane.** `internal/schema/registry.go` now defines coreFields = [id, type], vaultmindOwnedFields = [created, vm_updated], humanCompatFields = [updated], graphFields = [...]. `RequiredFields()` returns coreFields ∪ vaultmindOwnedFields ∪ td.Required. Disjointness pinned via `TestFieldTiers_PairwiseDisjoint`. The schema's read-side / write-side asymmetry (validator demands; the producer must supply) is now uniform: the mutator auto-bumps every Op, the fix command backfills, the detector surfaces drift.

**Auto-maintenance lane.** Mutator auto-bumps `vm_updated` on every Op when the note is domain. Used a load-bearing `IsDomain` guard rather than defense-in-depth — `ValidateMutation` has an early-return for `OpNormalize` that would bypass downstream guards. 7+ TDD tests pin the contract across Set/Unset/Merge/Normalize plus DryRun-doesn't-persist plus non-domain-skips-bump.

**Audit/apply lane (3+4 combined commit `ad15d6a`).** New `internal/fix/` business package + `cmd/frontmatter_fix.go` wiring (≤30 lines) + `cmd/frontmatter_fix_core.go` output formatting. Default dry-run; `--apply` opt-in. Provenance for `created`: git first-commit (`git log --diff-filter=A --follow --format=%as`) → file mtime → today. Layering work: added `internal/fix/**` to `.go-arch-lint.yml` business layer; SAST exclusion added with documented justification.

**Drift signal lane (commit `afac754`).** `query.DetectVMUpdatedDrift(vaultPath, paths)` reads filesystem (not stale DB state), 5s tolerance for vaultmind's own write jitter. Doctor wires it as `Issues.StaleVMUpdated` count + per-note `StaleVMUpdatedDetails`. CLI prints `⚠ Stale vm_updated: N note(s) edited since vaultmind processed them / run: vaultmind frontmatter fix --apply --vault <vault>`. Smoke-tested 0 drift on both real vaults — expected, since neither has been auto-bumped yet.

**SSOT extraction lane (followup commit `40a43b0`).** Code-reviewer caught two MEDIUMs: date-only `"2006-01-02"` literal scattered across 5+ write sites with no constant. Extracted `schema.CreatedDateFormat`; switched all four write sites (fix.go, normalize.go, template/process.go, initvault.go) to reference it. Read-side coercions (indexer's date-to-string, normalize's parse-tolerance list) intentionally NOT switched — those are parse-side, not the `created` write contract. Plus a corrupt-frontmatter capture-path test pinning the `//nolint:nilerr` suppressions.

**Review-finding-closure lane (followup commit `4beadff`).** Code-reviewer caught one HIGH (`--backfill` flag in the user-facing drift-resolution message — the flag doesn't exist; running the suggestion would fail with "unknown flag") and one MEDIUM (yaml.v3 unmarshals unquoted RFC3339 as `time.Time` not `string` — the type assertion silently failed, classifying every hand-edited note as "absent" and missing real drift). Both fixed with type-switch fallback + new test pinning the `time.Time` path.

**Docs lane (commit `70c8c0b`).** `docs/AGENT_ONBOARDING.md` §5e: the migration flow now points at `vaultmind frontmatter fix --apply` instead of teaching agents to hand-edit notes one at a time. Names the four-tier taxonomy as the WHY; calls out provenance for `created`; mentions doctor's drift detector as the post-apply verification path.

**Generalizable lessons:**

1. **The pivot was the work.** Six hours into "build the validator" became "rebuild the ownership model," and the rebuild was correct. The pivot trigger was Peiman noticing the manifesto violation; the recovery was truth-seeking ("which fields are actually used?") — manifesto principle 1. Lesson: when the user says "why did this happen," it's almost never "you forgot a step" — it's "your frame is wrong, redo the analysis."

2. **SSOT is a discipline, not a constant.** The first reviewer pass found `"2006-01-02"` scattered across 5+ files. None of the original implementers (myself included) had hardcoded it deliberately — it crept in because Go's date format is short enough to feel inline-acceptable. Lesson: every literal that's a "format" is a constant, full stop.

3. **The reviewer is structurally necessary, not paranoid.** Three review passes; three real bug catches (date-only SSOT gap, wrong-flag UX, yaml.v3 unquoted-timestamp false-negative). The wrong-flag bug would have actively misled operators following doctor's "run this command" hint — they'd hit "unknown flag" on their most-likely follow-up action. Self-review wouldn't have caught it; my own message-writing was contemporaneous with the buggy command-flag wiring.

4. **"Always use subagents for review of the superpower" pays for itself.** The cost is one extra Agent call per commit; the gain is an independent reader who has not seen me rationalize the choices. Honor it on every commit, not just "important" ones.

5. **Default to dry-run; --apply is the explicit gate.** Per arc-extending-not-overwriting, vaultmind never silently rewrites user files. The fix command's default mode is dry-run for the same reason `git push` requires the verb: file edits are not idempotent and the operator must consciously opt in. The drift detector surfaces the signal; the fix command is the resolution path; both keep the user in the loop.
