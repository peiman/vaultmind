# ADR-101: Command Taxonomy — Graph Under `memory`, Doctor as the Health + Heal Hub

## Status

Accepted

## Context

The CLI's surface had grown three overlapping seams that confused agents and humans alike:

1. **Graph traversal was split across two parents.** A top-level `links` command
   (`links out` / `links in` / `links neighbors`) overlapped with `memory` (`memory recall`,
   `memory context-pack`). `links out`/`links in` were the same query with opposite directions
   shipped as two commands; `links neighbors` (BFS) and `memory recall` (traverse + full
   frontmatter) did nearly the same thing under different parents. An agent reaching for "what's
   connected to this note" had to know which of two trees to enter.

2. **Vault health was split across `doctor` and `vault status`.** `doctor` diagnosed; `vault status`
   gave the cold-start counts + per-type breakdown. The `vault` parent existed only to host `status`.

3. **The diagnosis remedy pointed at vaporware.** `doctor` told users to repair broken links by running
   an unshipped `scripts/fix_wikilinks.py`, while the actual repair logic lived under a top-level
   `lint fix-links` — a third, unrelated parent. (`lint` collided conceptually with `task lint`
   / `golangci-lint`, the CI tooling.)

The result was three ways to ask "what's near this note", two ways to ask "is my vault healthy",
and a fix-it instruction that named a script that does not exist.

## Decision

**1. Unify all graph traversal under `memory`.** The top-level `links` parent is removed.

- `memory links <id> [--out|--in|--both]` — directed wikilink edges, default `--both`; `--in` =
  backlinks. One direction-flagged command absorbs `links out` (outbound) and `links in` (inbound).
- `memory neighbors <id> [--depth N]` — multi-hop neighborhood with full frontmatter. Merges
  `links neighbors` (BFS) and `memory recall` (traverse + full frontmatter) into one.
- `memory pack <id> [--budget N]` — rename of `memory context-pack` (identical behavior).
- `memory related`, `memory summarize` — unchanged.

**2. Make `doctor` the single vault-health hub.**

- `doctor` — read-only diagnosis, and it now carries the per-type breakdown `vault status` had.
- `doctor --summary` — the cold-start view (counts + per-type breakdown + errors/warnings rollup);
  this is what `vault status` produced.
- Repair lives under doctor: `doctor heal` applies every auto-fixable repair doctor found (today:
  wikilinks); `doctor heal wikilinks` is the surgical form (logic moved from `lint fix-links`).
  `heal` is the canonical verb; `fix` is a permanent cobra alias (`doctor fix`, `doctor fix wikilinks`;
  help shows "heal (fix)"). `heal` **applies by default**; `--dry-run` previews. All `doctor heal *`
  paths share one engine (`internal/mutation`).
- The `vault` parent dissolves (deprecated). Top-level `lint` is removed; there is **no** top-level
  `fix`/`heal`. `dataview lint` is a separate domain checker and stays.

**3. Fix the seam-3 remedy.** `doctor`'s broken-links remedy now points at
`vaultmind doctor heal wikilinks` instead of the unshipped `scripts/fix_wikilinks.py`.

**Deprecation policy (in scope; ~2 releases).** Every removed/renamed invocation
(`links out`/`in`/`neighbors`, `lint`, `lint fix-links`, `vault status`, `vault`, `memory recall`,
`memory context-pack`) becomes a **hidden deprecated command** that prints a one-line stderr
deprecation notice and delegates to the new path by calling the **same internal function**. They are
kept for roughly two releases, then removed.

**Out of scope.** No behavior changes beyond this re-wiring — the refactor is re-wiring + aliasing,
reusing the existing `internal/query`, `internal/memory`, `internal/mutation`, and `internal/graph`
functions, not reimplementing them.

## Alternatives Considered

### Alternative 1: Keep `links` as its own parent, just dedupe `recall`/`neighbors`

**Pros:** Smaller diff; `links` is a familiar noun.

**Why not chosen:** It leaves two graph parents. Agents still have to learn which tree holds which
traversal, and the `links out`/`links in` two-command split for one directional query remains.

### Alternative 2: Keep `lint`/`vault` parents and just retarget the remedy string

**Pros:** Minimal change; fixes the vaporware remedy alone.

**Why not chosen:** It preserves `vault` (a parent whose only child is `status`) and `lint` (which
collides with the CI `task lint` mental model and hosts repair logic that belongs next to the
diagnosis that surfaces it). Co-locating heal with doctor is the SSOT win.

### Alternative 3: `fix` as the canonical verb, `heal` as the alias

**Pros:** `fix` is the more common CLI verb.

**Why not chosen:** `fix` collides with the `lint fix-links` and `task lint --fix` lineage we're
moving away from, and reads as a flag-style action. `heal` is distinctive, reads as "restore the
vault to health", and pairs cleanly with the `doctor` metaphor. `fix` stays as a permanent alias so
muscle memory and the deprecated `lint fix-links` path still resolve.

## Consequences

### Positive

- One place to traverse the graph (`memory`) and one place to check + repair vault health (`doctor`).
- The fix-it instruction now names a command that exists and works.
- Repair shares a single mutation engine, so new `doctor heal *` repairs reuse it rather than forking.
- Smaller, more discoverable root help.

### Negative

- Breaking surface change for anyone scripting the old invocations until the deprecated aliases are
  removed (~2 releases). Mitigated by hidden aliases that delegate to the same internal function.
- Two verbs for one repair (`heal` + `fix` alias) is mild surface duplication, accepted for migration.

### Neutral

- Help now shows "heal (fix)"; docs, hooks, and onboarding prose reference the new forms.

## References

- CHANGELOG `[Unreleased]` — Added / Changed / Deprecated entries for this taxonomy.
- ADR-001 (ultra-thin commands) and ADR-002/005 (config SSOT) — the conventions the refactor preserves.

---

**Decision Date:** 2026-06-07
**Decision Makers:** VaultMind maintainer
