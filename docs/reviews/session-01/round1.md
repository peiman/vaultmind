# Expert Panel — Round 1: Independent Reviews

**Date:** 2026-04-03 | **Document reviewed:** VaultMind SRS v3

---

## Panel Members

| # | Expert | Specialty |
|---|--------|-----------|
| 1 | Dr. Elena Vasquez | Cognitive neuroscience, long-term memory |
| 2 | Marcus Chen | Obsidian power user, vault architecture |
| 3 | Jordan Blackwell | Devil's advocate, systems architecture |
| 4 | Dr. Priya Sharma | Knowledge graphs, graph databases |
| 5 | Alex Novak | AI agent systems engineering |
| 6 | Sam Torres | CLI/DX design |

---

## 1. Dr. Elena Vasquez — Cognitive Neuroscience Review

### Alignment with Memory Science

- **Spreading activation analog:** The `memory recall` with configurable `--depth` and `--min-confidence` is a reasonable computational analog to spreading activation. The confidence gradient maps to associative strength.
- **Graduated evidence:** Three-tier confidence with source attribution reflects sound distinction between strongly and weakly encoded associations.
- **Alias/surface-form variation:** Acknowledges that the same concept may be referred to by different surface forms — real phenomenon in semantic memory.
- **IDF-weighted tag overlap:** Reasonable proxy for the specificity principle in semantic memory.

### Missing Memory Concepts

- **Forgetting and temporal decay:** Largest gap. No use of timestamps in retrieval ranking. A note from 2 years ago and one from yesterday receive identical treatment. The context-pack sorts by `updated` but this is a display heuristic, not a principled accessibility model.
- **Memory consolidation:** No mechanism for strengthening associations based on retrieval frequency. Every edge is static from creation.
- **Retrieval-induced forgetting:** Recall is purely additive — surfacing one node never suppresses competing neighbors.
- **Encoding specificity:** No concept of retrieval context. Same query always produces same result regardless of agent's current work.
- **Emotional salience:** No importance signal beyond linking frequency (which is untracked).
- **Prospective memory:** No "remember to do X at time Y" structures.
- **Schema/frame effects:** Retrieval is pure graph traversal with no schema-level inference.

### Misleading Metaphors

- **"Memory recall"** is deterministic graph traversal, not reconstructive recall.
- **"Confidence"** is actually source provenance (how the edge was created), not epistemic uncertainty.
- **"Associative memory"** in the product name overpromises — no partial-cue retrieval or pattern completion. Requires a fully specified seed note.

### Recommendations

1. **Add recency-weighted retrieval score:** `accessibility = confidence_weight * e^(-λ * days_since_updated)`. Basis: ACT-R memory model.
2. **Track retrieval frequency:** Add `recall_count` and `last_recalled_at` to `notes` table. Basis: testing effect.
3. **Rename "confidence" to "edge_source" or "link_provenance."**
4. **Add contextual query mode** for `memory related` — caller supplies active context to bias retrieval.
5. **Add salience scoring** via inbound link count or author-set importance.
6. **Document what the system does NOT model** — prevent the branding from misleading developers.

---

## 2. Marcus Chen — Obsidian Practitioner Review

### Obsidian Compatibility

**Gets right:** Identity in frontmatter (not filenames), `.obsidian` excluded, atomic writes preserve file watcher, HTML comment markers invisible in reading view.

**Gets wrong or misses:**
- **`tags`/`tag` naming:** Obsidian supports legacy `tag` singular key. Many older vaults use it.
- **`updated` field collision:** Auto-update plugins (`update-time-on-edit`) will rewrite `updated` on every save, causing hash conflicts that refuse VaultMind writes. Day-one problem for power users.
- **`id` format:** `{type}-{slug}` looks like inline tag fragments. Not a hard bug but operators should know.
- **Mobile sync absent:** iCloud updates mtimes on download without content change. `--watch` via fsnotify will receive spurious events.

### Real-World Vault Friction

- **No migration path:** A 5,000-note vault with no `id` fields is entirely "unstructured" on day one. No `frontmatter backfill` command exists. Biggest adoption cliff.
- **Inline `#tags` not indexed:** Many users rely on inline tags in body text. `tag_overlap` only operates on frontmatter `tags`. Users get incomplete graph results.
- **Templater collision:** `{{variable}}` syntax is identical to Templater. Templater will auto-process VaultMind templates and corrupt placeholders.
- **Dataview `this.id`:** Generated regions contain Dataview queries that only execute in Obsidian. Agents see raw query text. The SRS slightly overstates "Dataview-compatible generated views."

### Frontmatter Concerns

- **YAML boolean coercion:** `yes`, `no`, `true`, `false`, `on`, `off` silently coerced by YAML 1.1 parsers. Needs explicit YAML 1.2 strict mode requirement.
- **Multiline block scalars:** `frontmatter normalize` key-reordering could corrupt `|` or `>` block scalars. Needs explicit round-trip preservation rule.
- **Normalize strips datetime precision:** Converting `YYYY-MM-DDTHH:MM:SS` to `YYYY-MM-DD` permanently loses time for meeting/journal notes. Should be opt-in.
- **Obsidian Properties panel:** Date-typed fields serialized differently. Generally compatible but needs guidance.

### Dataview Interaction

- **`FROM "projects"` hardcodes paths:** Real vaults rarely have this clean a structure. Templates need variable substitution.
- **Dataview inline fields (`field:: value`) not mentioned.** Should be stated as compatibility boundary.
- **`dataviewjs` edge detection unspecified.** Either clarify `dataview_reference` edges only come from standard blocks, or remove the claim.

### Recommendations

1. **Add `frontmatter backfill` command** for batch ID/type assignment.
2. **Change template syntax** from `{{variable}}` to avoid Templater collision.
3. **Add Obsidian-native key whitelist** (`cssclasses`, `publish`, `banner`, `alias`).
4. **Document `updated` field plugin conflict** in risks.
5. **Index inline `#tags`** from body text into `tags` table.
6. **Make datetime stripping opt-in** via `--strip-time` flag.
7. **Require YAML 1.2 strict mode** in implementation.

---

## 3. Jordan Blackwell — Devil's Advocate Review

### Contradictions and Inconsistencies

- **"Single source of truth" vs checksum model:** Vault is canonical, but VaultMind refuses writes when its checksum disagrees with vault content. The vault is the source of truth until VaultMind disagrees.
- **Stale preview in mutation workflow:** Preview (step 3) generates diff before hash verify (step 5). Agent sees potentially stale diff before safety check fires.
- **Plan atomicity is best-effort undo:** Rollback via pre-operation file copies. If process killed mid-plan, partial writes with no rollback. Not truly atomic.
- **FTS sync has no crash recovery:** `fts_notes` is standalone. Crash between `notes` update and `fts_notes` update leaves them out of sync until full rebuild.

### Unstated Assumptions

- Single vault, single config, single Git repo, all co-located. No submodules, no multi-repo vaults.
- Vault contains only human-authored `.md` files. No symlinks, no binary `.md` files, no plugin-generated indexes.
- YAML is well-formed UTF-8. No handling of anchors, merge keys, multi-document separators.
- Token budget estimation (1 token ~ 4 chars) is wrong for code, CJK text, base64.
- `index_hash` is non-deterministic with SQLite WAL mode.
- `--watch` concurrent with mutations violates single-writer assumption.

### Scope and Complexity Risks

- Phase 1 is too large: vault scanner + 6-link-type parser + 8-table SQLite schema + FTS + entity resolution + all query commands + JSON output + type registry.
- Dataview generated regions add complexity for near-zero agent value.
- Alias mention detection is O(notes * aliases) per index run — may blow 30s target.
- `frontmatter normalize` snake_case conversion is underspecified and dangerous.

### Unhandled Failure Scenarios

- Corrupted/truncated YAML frontmatter
- Enormous files (50MB meeting notes)
- Non-UTF8 / binary content in `.md` files
- Symlink loops
- Vault inside vault (nested VaultMind configs)
- Plan `note_create` then `frontmatter_set` on same note (targets resolved before execution — impossible)
- Git hook failures leave dirty state that blocks next write

### The Hard Question

**Most likely failure: spec not validated against a real vault.** Every decision was made in the abstract. Before writing code, index one real messy vault and record every assumption violation. Build scanner and parser first, run on real data, let failures rewrite the spec.

---

## 4. Dr. Priya Sharma — Knowledge Graph Review

### Graph Model Assessment

- **Edge taxonomy mostly sound.** Six types correctly separate structural from inferred signal.
- **`explicit_embed` redundant:** Carries identical traversal information to `explicit_link`. Should merge with `target_kind: embed` attribute.
- **`dataview_reference` misclassified at `low`:** Path/ID-based Dataview references are author-asserted, should be `medium`.
- **Three confidence levels insufficient:** All `high` edges (link, embed, relation) are conflated. Agents can't distinguish `parent_id` from casual wikilink. Need numeric `weight` on all edges.
- **No `is_inferred` flag:** Consumers must maintain lookup table of which types are inferred. Add computed column.

### Entity Resolution

- **Sound strategy with fragile alias growth.** Five-tier cascade correct. `ambiguous: true` exactly right.
- **Alias prefix collision:** Short common words as aliases (e.g., "API", "DM") need per-alias scope or suppression list.
- **Unbounded alias growth:** No limit, no deduplication, no staleness mechanism. False-positive factory over time.
- **Missing `title_normalized` column:** Asymmetry with `alias_normalized` in resolution tier 4.

### SQLite as Graph Store

- **Correct choice for scale and deployment model.** Rebuild-from-vault removes durability argument.
- **Missing indexes:** `links(edge_type)`, `links(confidence)`, `links(src_note_id, resolved)`, `notes(type)`, `aliases(note_id)`.
- **Depth-2 BFS in SQL:** Recursive CTE with fan-out 50 = 2,500 rows before filtering. 500ms target achievable with composite indexes.

### Traversal and Recall

- **Cycle detection not specified.** `related_ids` and wikilinks can form cycles. Must mandate visited-set BFS.
- **Fan-out explosion:** No `--max-fan-out` or `--max-nodes` cap. Depth-2 from hub = O(vault_size²) worst case.
- **BFS should be mandated** (distance tracking requires it).
- **Add `via_id`** to depth-2+ nodes for path reconstruction.

### Recommendations

1. Add `is_inferred` computed column on `links`.
2. Add four missing indexes.
3. Mandate visited-set BFS for all depth>0 traversals.
4. Add `--max-nodes` cap (default 200).
5. Add `via_id` to recall response nodes.
6. Raise `dataview_reference` confidence for path-based references.
7. Add `title_normalized` to `notes` table.
8. Add alias cap or staleness warning.

---

## 5. Alex Novak — Agent Systems Review

### Agent Ergonomics

- **8-command core for 90% of agent work** — genuinely good surface area.
- **`--json` is a footgun:** Must default when stdout is not TTY. Human output should be opt-in.
- **`note get` always returns full body:** Needs `--frontmatter-only`. Agents doing bulk sweeps burn context on prose.
- **No batch-read:** Five resolves = five `note get` calls. Need `note mget`.

### Context Window Efficiency

- **`context-pack` is well-designed.** Packing algorithm, budget, `budget_exhausted` flag — all good.
- **`memory recall` payload ambiguous:** Unclear whether nodes include body text. Must explicitly guarantee frontmatter-only.
- **`meta` envelope wastes ~100 tokens per call.** Need `--no-meta` option.

### Agent Workflows

- **"Find and update" workflow:** Acceptable round-trips but `apply` requires filesystem path. Need `--stdin`.
- **"Create decision record":** Well-supported, 1-2 round-trips.
- **"Answer question about vault":** Best-designed workflow. search + context-pack = 2 commands, bounded output.
- **`--field` list syntax ambiguous:** How to pass `related_ids` as list? Unspecified.

### MCP Server Potential

- **Maps cleanly to MCP tools.** Each subcommand becomes a tool.
- **Missing `list-types` command:** Agents need to discover type registry without reading config.
- **`meta.index_hash` useful for cache invalidation** in MCP context.

### Recommendations

1. Default `--json` when not TTY.
2. Add `--frontmatter-only` to `note get` and `memory recall`.
3. Add `apply --stdin`.
4. Define `--field` list-value syntax.
5. Add `schema list-types --json`.
6. Add `note mget` for batch reads.
7. Explicitly specify `memory recall` nodes never include body.
8. Add error codes to all refusal responses (not just mutations).

---

## 6. Sam Torres — CLI/DX Review

### Command Structure

- **`resolve` and `index` are orphan verbs** at root level while everything else is namespaced. Should be `note resolve` or `vault resolve`.
- **`links neighbors` vs `memory recall` distinction unclear** at command level. Consider renaming `links neighbors` to `links graph`.
- **Missing `note list`:** Users will reach for it before `search`.

### Flag Design

- **`--diff` without `--dry-run` is a footgun:** Shows diff then still executes. `--diff` should imply `--dry-run`, or merge into single flag.
- **`--allow-extra` missing from `frontmatter merge`.**
- **`--force` should be `--override-checksum`** — more specific about what's being forced.
- **Missing `--no-commit`** as escape hatch for config-level commit defaults.

### Composability

- **JSON envelope slightly hostile to jq.** Always need `.result.` prefix. Consider `--unwrap` flag.
- **`search` is well-designed for pipes.** ID-first responses pay off.
- **Exit codes correct** (0/1/2).
- **`--watch` produces no structured events.** Need JSON lines to stdout on each cycle.

### Progressive Disclosure

- **11 top-level items with no grouping.** Need category headers: Query, Notes, Graph, Mutations, Ops.
- **`doctor` should be the entry point,** not a footnote.
- **`--dry-run` pattern is great** for progressive disclosure.

### Error Experience

- **Error codes inconsistent.** Mutations have codes; search/resolution don't.
- **`errors` array contains bare strings.** Should be structured: `{code, message, field}`.
- **Ambiguous resolution should populate `result`** even when `status` is `error`.

### Recommendations

1. Merge `--diff` into `--dry-run`.
2. Structured error objects in envelope.
3. Namespace `index` under subcommand (`index build`).
4. Category groups in `--help` output.
5. JSON events for `index --watch`.
6. Add `--allow-extra` to `frontmatter merge`.
7. Rename `--force` to `--override-checksum`.
8. Add `note list` command.
