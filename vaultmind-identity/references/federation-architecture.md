---
id: reference-federation-architecture
type: reference
title: "Federation Architecture — Cross-Vault Retrieval as a Lighter Read-Only Path"
created: 2026-05-02
vm_updated: 2026-05-02
tags:
  - federation
  - architecture
  - retrieval
  - plasticity
  - paper-2
related_ids:
  - reference-plasticity-priority-order
  - reference-paper-federated-constants
  - reference-onboarding-ax-design
  - arc-the-lighter-move-is-the-work
---

# Federation Architecture — Cross-Vault Retrieval as a Lighter Read-Only Path

## Why this exists

Two distinct problems converge on the same architectural answer:

1. **Tightly-connected projects** (e.g., the Daana ecosystem — content-machine, data-platform, blog) want one associative-memory experience across multiple repositories without duplicating shared concepts. The mega-vault alternative (one physical vault, all projects symlinked, mandatory id-prefix discipline) requires significant new schema and validator work for namespaces and type-collision handling. Federation avoids most of those costs.
2. **Paper #2 — Federated Retrieval-Constant Tuning** (`reference-paper-federated-constants`) requires federation infrastructure as the empirical substrate. The paper aggregates per-vault variant-performance signals across a population of vaults; that aggregation pipeline is the same per-vault-fingerprint mechanism a consumer-facing federation reads from.

Federation read (cross-vault retrieval) is **lighter than mega-vault** for the product feature AND is the substrate the paper depends on. One architecture, two motivations.

## The architectural fork — recap

| Concern | Mega-vault (Arch I) | Federation (Arch II) |
|---|---|---|
| ID collision | Real, needs prefix discipline + validator | Impossible — vault-fingerprint disambiguates |
| Type collisions | Real, needs union or per-namespace types | Impossible — each vault has own registry |
| Embedding mixing | Real cross-domain false-positive risk | Avoided — per-vault embedding model |
| Path stability | Real (symlinks break) | Per-vault is self-contained; projects move freely |
| Cross-project queries | Native (one index) | Layer above per-vault retrieval |
| Shared concepts SSOT | Native | Either duplication or qualified references |
| New CLI surface | Minimal | Federation config, cross-vault RRF, qualifier resolver |

Federation is the right answer when projects can have their own self-contained vaults AND a few shared concepts deserve their own vault to be referenced from anywhere. Mega-vault is the right answer when projects are so tightly coupled that even shared-concept extraction feels like fragmentation.

For the Daana ecosystem and similar tight-but-not-fused project families, federation with a dedicated `shared` vault for cross-project concepts is the natural shape.

## Cross-vault retrieval mechanics

### Per-vault retrieval is unchanged

Each vault has its own `.vaultmind/index.db`, runs its own 4-way RRF (dense ⊕ sparse ⊕ colbert ⊕ fts), tracks its own access events. Federation does not change anything inside a vault.

### Federation layer sits above per-vault retrieval

Three responsibilities:

1. **Fan out** — take a query, send it to N configured vaults in parallel.
2. **Collect** — each vault returns top-K with per-lane RRF rankings.
3. **Merge** — combine across-vault results into a unified ranked list.

### Merge mechanism — cross-vault RRF (the lean answer)

Four candidates evaluated:

| Option | Mechanic | Apples-to-apples? | New deps |
|---|---|---|---|
| Score concat | Concat top-K with RRF scores, sort | ❌ scores not directly comparable across vaults | none |
| **Cross-vault RRF** | Treat each vault as one "lane" in higher-order RRF | ✅ rank-based, scale-free | none |
| Cross-encoder rerank | Second model (e.g. BGE-reranker) over candidates | ✅ but expensive | new model |
| LLM judge | Ask an LLM to rank candidates | ✅ but slow + non-deterministic | LLM call |

**Cross-vault RRF** is the choice. Same math vaultmind already uses internally, just promoted one level. Conceptually:

```
within vault A: dense ⊕ sparse ⊕ colbert ⊕ fts → ranked-A
within vault B: dense ⊕ sparse ⊕ colbert ⊕ fts → ranked-B
across vaults:  ranked-A ⊕ ranked-B → final ranked
```

Notes appearing in multiple vaults' top-K (because user has overlapping concepts) get amplified. Notes appearing in only one vault still get partial credit.

### Federation config

```yaml
# ~/.config/vaultmind/federation.yaml
vaults:
  - name: persona
    path: ~/.vaultmind/persona
    role: identity                # privileged — always queried for identity questions
  - name: shared
    path: ~/.vaultmind/shared     # cross-project concepts SSOT
    role: shared
  - name: shahname
    path: ~/dev/siavoush/shahname-rts
    role: project
  - name: cm
    path: ~/dev/daana/daana-content-machine
    role: project
```

### CLI shape

- `vaultmind ask "X"` (no flag) — federates across all configured vaults if `federation.yaml` exists; otherwise falls back to today's single-vault behavior.
- `vaultmind ask "X" --vault shahname` — scopes to one named vault (today's behavior, preserved).
- `vaultmind ask "X" --vaults persona,shahname` — explicit subset.

### Cross-vault references — qualifier syntax for `related_ids`

A bare `related_ids: [concept-rrf]` is ambiguous when both vaults could have that id. Qualified `related_ids: [shared:concept-rrf]` is explicit and stable. Resolution: parse qualifier, route to named vault. Unqualified ids resolve to the local vault first, then fall back to federation (with ambiguity-error if multi-vault hits).

The vault fingerprint (commit `101b129`, `vaultmind-vault/.vaultmind/fingerprint.txt`) is the canonical disambiguator. Federation config maps human-readable names to fingerprints. Internal storage uses fingerprints; UI/files use names.

### Output annotation

Each hit is prefixed with `[vault-name]` so the agent/user sees origin:

```
[shared] concept-rrf — Reciprocal Rank Fusion (0.041, strong)
[shahname] research-shahnameh-rts — Shahnameh RTS Research and Scoping (0.037, moderate)
```

Removes the "why is this here?" confusion. Free UX once the federation merge is in place.

## Reranking — three layers

Increasing cost, decreasing frequency. Layer-clean separation: each layer is independently testable and deletable.

```
Layer 1 — Per-vault hybrid (always runs)
  Dense ⊕ sparse ⊕ colbert ⊕ fts → 4-way RRF
  Lives in: internal/retrieval/, internal/query/
  Federation-agnostic.

Layer 2 — Cross-vault merge (always runs in federated mode)
  Per-vault top-K → cross-vault RRF
  Lives in: NEW package internal/federation/
  Composition only — does not touch per-vault internals.

Layer 3 — Cross-vault rerank (deferred — opt-in feature)
  Top-K final candidates → cross-encoder OR LLM judge
  Lives in: same federation package, opt-in flag
  Use case: high-stakes queries where Layer 2 ambiguity matters.
```

### Calibrated confidence in federation

Single-vault calibrated confidence (plasticity step 4, first slice landed) is the score gap between top-1 and top-2. In federation, a richer signal exists: "vault A's top-1 dominates with confidence `strong`, but vault B has a competing top-1 with overlapping content — this is moderate cross-vault, even if it was strong within-vault."

That's a federation-aware calibration signal that doesn't exist single-vault. Layer 2 already has the per-vault ranks needed to compute it; surfacing it through `TopHitConfidence` is a small follow-on once Layer 2 ships.

## Plasticity — where it lives

This is the most interesting question. Three architectural shapes:

### A. Per-vault plasticity, federation is purely query-time merge

Each vault tracks its own access events. A note read in vault A doesn't affect vault B's activations.

- ✅ Clean separation; each vault is autonomous.
- ❌ A user who works mostly in vault A but has shared concepts in vault B finds vault B's hot list never updates from cross-vault queries.

### B. Federation-level plasticity (cross-vault working memory)

A separate "working memory" layer above all vaults tracks access events globally. Activation-triggered recall surfaces vault-agnostically.

- ✅ Reflects the human's actual working set regardless of where notes physically live.
- ❌ New persistent state. Where does it live (`~/.vaultmind/federation-memory.db`)? Adds a sync concern. Splits SSOT.

### C. Hybrid — per-vault primary, federation cache for surfacing (the right shape)

Each vault's `last_accessed_at` and `access_count` stay primary — single source of truth, lives in the vault that owns the note. The federation maintains a lightweight in-memory cache of recent cross-vault accesses — `(vault_fingerprint, note_id, timestamp)` tuples — used at query time to inform activation-triggered recall across vaults. Cache is rebuilt from per-vault primaries on session start; doesn't need its own persistence.

- ✅ SSOT preserved. No new persistent state. Federation-level surfacing without federation-level state.
- ✅ Matches how human memory seems to work: episodic centralization, content distribution.

**This is the shape we ship.** Decay/reinforcement (plasticity step 5) stays per-vault. Surfacing (activation-triggered recall, step 3) federates.

### Where activation-triggered recall fires

The SessionStart hook today runs `vaultmind ask "what matters most right now"` against the persona vault. Federated, it runs against ALL configured vaults, with the federation merge surfacing identity (persona) + current-context (project) + research-relevant (research) in one unified context-pack. The federation IS the activation-triggered recall layer for cross-vault.

## Where federation fits in the plasticity roadmap

Per `reference-plasticity-priority-order`:

```
1. Episodic substrate                          ✅ shipped 2026-04-24
2. Arc distillation                            (NEXT)
3. Activation-triggered recall                 ✅ slices landed
4. Calibrated confidence                       ✅ first slice landed
5. Decay + reinforcement                       in progress (slice 5b' degrading)
5.5 (NEW). Federation read                     this document
6. MCP / cross-agent memory                    write — deferred
```

Federation read is **lighter than step 6**. Step 6 is "writing to other minds" — cross-agent collaboration with mutation. Federation read is "querying multiple memories" — read-only, no mutation, no multi-agent state.

Step 5.5 unblocks Paper #2's empirical work without committing to step 6's complexity.

## Connection to Paper #2

Paper #2 (`reference-paper-federated-constants`) thesis: retrieval constants generalize across personal vaults via federated aggregation. The paper's hypotheses are testable ONLY if you have:

- Multiple users, each with their own vault.
- An aggregator that receives variant-performance metrics (not content).
- Vault fingerprints for anonymous per-vault attribution. ✅ shipped (commit 101b129).

The federation we describe here is the **consumer-facing version** of that infrastructure. Same fingerprint mechanism. Same per-vault isolation. The only difference: paper-time aggregation goes to a remote server with consent + DP; user-time federation merges locally.

So building federation produces:
- **Product feature**: cross-vault retrieval for users with multiple knowledge bases.
- **Research substrate**: same code path can drive Paper #2's data collection (with consent + DP).

Two motivations, one architecture.

## Minimum viable slice — probe-shaped first version

Per `arc-the-lighter-move-is-the-work`: ship the lightest sufficient version. Defer everything that doesn't surface real friction.

**Phase 0 — pre-implementation probe.**
Take the existing two vaults (`vaultmind-identity` and `vaultmind-vault`) and mentally federate them: same query against both, what would the merged result look like? Does it surface useful blends? Today vaultmind has no `--vaults` flag — the probe is an analytical thought experiment, not running code yet.

**Phase 1 — the first slice.**

1. **Config**: `~/.config/vaultmind/federation.yaml` defining named vaults.
2. **CLI**: `vaultmind ask "X"` reads federation config; queries each vault; merges via cross-vault RRF; returns unified list. `--vault <name>` scopes to one (preserved). `--vaults a,b` for explicit subset.
3. **Merge**: cross-vault RRF (same math, one level higher). New package `internal/federation/`.
4. **Output**: each hit annotated with `[vault-name]` prefix.
5. **References**: `related_ids` accepts `vault-name:id` qualifier; resolver routes by qualifier.
6. **Plasticity**: per-vault unchanged. No federation-level memory cache yet (Hybrid shape's cache deferred until measurement shows it's load-bearing).
7. **Rerank**: no Layer 3 cross-vault rerank yet.
8. **Backwards compatibility**: in absence of `federation.yaml`, all behavior identical to today.

**Phase 2 — what gets earned by Phase 1's data.**

- Federation cache for activation-triggered recall (Hybrid shape — only if missing-cross-vault-warmth becomes a felt friction).
- Layer 3 cross-vault rerank (only if cross-vault ambiguity becomes load-bearing).
- Federation-aware `TopHitConfidence` (small follow-on, low cost).
- Cross-vault graph traversal (`parent_id` across vaults — only if shared-vault-as-parent becomes a real pattern).

**Real probe corpus**: today's `vaultmind-identity` (33 notes, persona) + `vaultmind-vault` (407 notes, research). Heterogeneous, real, both already in this repo. The probe is a 2-vault federation that already exists — no new corpus needed.

## Open design questions

1. **Where does federation config live — global or per-project?**
   - Global at `~/.config/vaultmind/federation.yaml` is per-user.
   - Per-project at `<project>/.vaultmind/federation.yaml` lets each project define its own scope.
   - Not mutually exclusive, but precedence rule matters. My instinct: per-user as default; per-project override available; precedence is project > user.

2. **What's the persona vault's privilege?**
   - "Persona" is structurally different from "knowledge" — it's identity, not domain content.
   - Should federation always include the persona vault implicitly (regardless of config)?
   - The `role: identity` flag in the config sketch is one answer; another is hard-coding "always include the user's persona vault."
   - My instinct: declare it via `role` in config; vaultmind treats `role: identity` as always-included.

3. **What about identity-typed queries specifically?**
   - "Who am I?" should route preferentially to identity-role vaults.
   - "How do I work?" — same.
   - Domain queries should NOT be biased toward identity vault.
   - Could be: every query queries every vault, but identity-role vault gets a rank boost when the query embeds close to identity-typed content.
   - Defer — measurement first.

4. **Cross-vault writes.**
   - Step 6 territory. Out of scope for federation read.
   - But: when an agent generates a new arc during a session, where does it land? Persona vault? Project vault?
   - Today's CLI assumes single-vault writes. Federation-aware write needs a routing decision (where does this new note belong?). Probably interview-driven by the agent.

5. **Per-vault embedding model variation.**
   - Today's federation assumes same model across vaults (BGE-M3).
   - Mixing embedding models across federated vaults is a real concern (cross-vault RRF on rank is OK; cross-vault embedding-distance comparison is not).
   - Defer until a real use case (e.g., a vault using MiniLM falls back path on a different ORT setup).

6. **Federation telemetry granularity.**
   - For Paper #2, per-vault rollups (`vault_fingerprint, variant_id, Hit@K, MRR`) are the substrate.
   - Federation adds: cross-vault rollups (`(vault_fingerprint_pair), federation_rank, federation_score`).
   - Cross-vault telemetry shape is a separate design pass once Phase 1 ships.

## What this is not

- **Not mega-vault.** Federation explicitly avoids the namespace/type-collision discipline mega-vault would require.
- **Not cross-agent memory (step 6).** Federation read is read-only. Step 6 is write-capable.
- **Not multi-user.** Federation across multiple personal vaults of the SAME user. Multi-user would require trust boundaries, write permissions, conflict resolution — out of scope.
- **Not a federated-learning / federated-aggregation system.** That's Paper #2's substrate (already shipped via `vaultmind export --rollup`). Federation here is retrieval-time merge for the local user.

## Source

- Conversation date: 2026-05-02.
- Originating question: "tell me more about Arch II (federated). how would cross vault work and where would the reranking and plasticity live?"
- Conversation transcript: `~/.claude/projects/-Users-peiman-dev-cli-vaultmind/<this-session-id>.jsonl` (auto-captured at SessionEnd to `vaultmind-identity/episodes/episode-2026-05-02-*.md`).
- Companion arc: `arc-the-lighter-move-is-the-work` (the discipline this design honors — probe-before-commit at scope, ship the lightest probe first).
- Companion paper: `reference-paper-federated-constants` (Paper #2, the research substrate this architecture also serves).
- Companion roadmap: `reference-plasticity-priority-order` (federation read = step 5.5, between decay/reinforcement and MCP write).
- Companion plan: `reference-onboarding-ax-design` (the new-user AX work that surfaced the tightly-connected-projects use case).
- Implementation prerequisite (already shipped): vault fingerprint (`vaultmind-vault/.vaultmind/fingerprint.txt`, commit `101b129`).
