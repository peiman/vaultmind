# Session 02 — Summary of Findings

**Date:** 2026-04-03 | **Spec:** SRS v3 (post-fix, incorporating Session 01 findings) | **Panel:** 7 experts

> Full transcripts: [Round 1](round1.md) | [Round 2](round2.md) | [Panel roster](panel.md)

---

## Session 01 Fix Assessment

The panel confirmed that all 10 Session 01 consensus findings were addressed. Specific verdicts:

| Fix | Verdict |
|-----|---------|
| `--json` auto-detect via TTY | Correctly implemented |
| Structured error objects | Correctly implemented |
| Plan create-then-configure sequencing | Correctly fixed (per-operation resolution) |
| Template syntax `{{}}` → `${}` | **Partially fixed** — `${variable}` still collides with Templater JS mode |
| BFS cycle detection | Correctly mandated |
| `--frontmatter-only` on note get | Correctly added |
| `--max-nodes` cap | Correctly added |
| Missing database indexes | All 5 added |
| Uniqueness constraint on links | Added with delete-before-reinsert contract |
| Confidence/provenance clarification | Adequately documented |

---

## New Consensus Findings (5+ experts agree)

### 1. INDEX STALENESS — New #1 Failure Mode
**Raised by:** Blackwell | **Endorsed by:** all 6 others
After any mutation, the SQLite index is stale. `memory recall`, `links neighbors`, and `context-pack` return pre-mutation state with no staleness signal. An agent doing read-modify-write-recall acts on a lie. `meta.index_hash` doesn't help (it's the DB file hash, changes on any rebuild, not correlated to specific mutations).
**Consensus fix:** Post-write implicit incremental re-index of affected file(s), OR a `reindex_required: true` flag in mutation responses + auto-reindex on next read command.

### 2. `write_hash` in All Mutation Responses
**Raised by:** Nakamura | **Endorsed by:** Vasquez, Chen, Novak, Hoffmann, Blackwell
No mechanism for agents to confirm whether a mutation landed. After a timeout, the agent cannot distinguish success from failure. Every mutation response must include the post-write file SHA-256 so agents can verify.
**Also needed:** Plan-level `idempotency_key` field to prevent double-application on retry.

### 3. `updated` Field Must Be Resolved
**Raised by:** Novak (Session 01) | **Re-escalated by:** Novak, Chen, Vasquez, Nakamura, Hoffmann
The field remains `updated` in the schema spec (`04-frontmatter-schema.md`). The risk is documented but the schema is unchanged. Obsidian auto-update plugins will rewrite it, breaking hash-based conflict detection.
**Consensus fix:** Add `vm_updated` as the VaultMind-managed field. Leave `updated` alone (for Obsidian plugins). Document in schema spec, not just risks.

### 4. Template Delimiter Still Collides
**Raised by:** Chen | **Endorsed by:** Blackwell, Nakamura, Hoffmann
`${variable}` collides with Templater's JavaScript expression mode. The rationale text in `19-template-spec.md` is self-contradicting (claims to avoid Templater collision while using Templater syntax).
**Consensus fix:** Change to a truly unique delimiter: `<%=variable%>`, `«variable»`, or `{%variable%}`.

### 5. `max_nodes_reached` Flag in Recall Response
**Raised by:** Sharma | **Endorsed by:** Novak, Nakamura, Hoffmann, Blackwell
`memory recall` with `--max-nodes` truncates the result set but the response has no signal. Agents reason over incomplete neighborhoods as if complete.
**Fix:** Add `max_nodes_reached: boolean` to recall response shape. Mirror `budget_exhausted` pattern from context-pack.

### 6. Structured Warnings (not just Errors)
**Raised by:** Nakamura | **Endorsed by:** Novak, Hoffmann, Blackwell ("uncontested — write it into SRS")
`warnings` array contains bare strings. Agents can't branch on warning type. Must match error object structure: `{code, message, field}`.

### 7. Context-pack Sort Order: Weight Then Recency
**Raised by:** Vasquez | **Endorsed by:** Chen, Hoffmann, Novak
Sorting by `updated` desc surfaces recently-edited notes regardless of relevance. Should sort by edge weight (where available) then `updated` as tiebreaker. Schema already supports this — `weight` column exists on `links`.

---

## Strong Findings (3-4 experts, no opposition)

### 8. `vault status` Cold-Start Command
**Raised by:** Nakamura | **Endorsed by:** Novak, Blackwell
Combines `doctor` + `schema list-types` + `git status` + index freshness into one call. Eliminates 4-round-trip bootstrap tax.

### 9. Default `--max-nodes` Too High
**Raised by:** Nakamura (25) | **Debated:** Blackwell (50), Sharma (split render/traverse)
200 is too high for agent context windows. Consensus around 50 as default. Sharma's split (render vs traverse caps) is architecturally clean but adds complexity.

### 10. `note get` Should Default to Frontmatter-Only
**Raised by:** Nakamura | **Endorsed by:** Chen, Novak, Blackwell
Body text is the main context-window cost. Agents almost never need body on bulk sweeps. Default body-off, require `--include-body` to opt in.

### 11. Alias Mention Occurrence Count as Weight
**Raised by:** Vasquez | **Endorsed by:** Chen | **Opposed by:** Blackwell (noise concern)
Currently binary (one edge per pair). Storing occurrence count as `weight` unlocks meaningful sort within medium-confidence tier.
**Verdict:** Worth adding with clear documentation that weight = occurrence frequency, not semantic importance.

### 12. NULL Uniqueness Index on Unresolved Edges
**Raised by:** Sharma | **Endorsed by:** Chen, Blackwell
SQLite `NULL != NULL` means the unique index doesn't prevent duplicate unresolved edges. Fix: partial unique index with `WHERE dst_note_id IS NULL`.

### 13. Self-Edge Prevention
**Raised by:** Sharma | **Endorsed by:** Chen, Blackwell
Add `CHECK (src_note_id != dst_note_id)` or filter at insert time.

### 14. BFS Confidence Filter Ordering
**Raised by:** Sharma | **Endorsed by:** Vasquez
Filter before cap (during BFS expansion), not after. Filtering after produces topologically correct but epistemically distorted neighborhoods.

### 15. Missing Response Shapes (mget, apply)
**Raised by:** Novak | **Endorsed by:** Nakamura
`note mget` and `apply` have no documented response shapes in `09-response-shapes.md`. Agent contract gap.

---

## Contested Findings

### Collapse Memory Commands (5→3)
**For:** Nakamura, Novak | **Against:** Blackwell
Nakamura: merge `links neighbors` into `memory recall` with `--rich` flag, deprecate `memory related`. Blackwell: distinct problems need distinct tools, collapsing recreates complexity via flags.
**Verdict:** Keep all commands for v1. Add documentation clarifying when to use each. Revisit in v2 with usage data.

### Agent Access Log / Recency Tracking
**For:** Hoffmann, Vasquez, Nakamura | **Against:** Blackwell (violates derived-data principle)
Hoffmann: access log is load-bearing for retrieval quality. Blackwell: access patterns are agent behavior, not vault truth.
**Verdict:** Add to v2 roadmap as optional derived data (stored in SQLite alongside index, not in vault).

### Salience/Importance Field in Frontmatter
**For:** Hoffmann, Vasquez | **Caution:** Chen (must live in frontmatter, not SQLite-only)
**Verdict:** Add as optional domain-tier field in v2. Must be frontmatter (not hidden state) to respect vault-is-truth principle.

### Semantic Expansion / Embeddings Extension Point
**For:** Hoffmann | **Caution:** Blackwell (out of scope), Sharma (must be separate edge layer)
**Verdict:** Defer to v2. Define the interface now (Hoffman's recommendation) so v1 schema decisions don't prevent it.

---

## New Expert Contributions

### Kai Nakamura (AX Designer) — Key Unique Findings
- `vault status` cold-start command (consolidate 4 bootstrap calls into 1)
- `write_hash` mutation receipt for retry safety
- Plan `idempotency_key` for safe re-submission
- `frontmatter merge` list conflict semantics undefined (union vs replace?)
- `meta.index_hash` is wrong staleness signal (DB hash, not vault content hash)
- `resolve` command redundant in MCP context
- `--commit` as hidden side effect problematic for MCP

### Dr. Lena Hoffmann (AI Memory Researcher) — Key Unique Findings
- VaultMind maps to MemGPT's archival memory tier; agents also need working memory
- Missing three retrieval signals from research: recency, importance, semantic similarity
- No query-relative ranking (retrieval is target-centric, not task-centric)
- No summarization/compression for context-pack efficiency
- Reflection note type for second-order memory (Reflexion pattern)
- Risk: agents create notes faster than they can be meaningfully retrieved without importance scoring

---

## Top 10 Action Items (Priority Order)

| # | Action | Severity | Effort | Source |
|---|--------|----------|--------|--------|
| 1 | Post-write index staleness signal (implicit re-index or dirty flag) | **Blocker** | Medium | Blackwell, all |
| 2 | `write_hash` in all mutation responses + plan `idempotency_key` | **Blocker** | Low | Nakamura, Novak |
| 3 | Add `vm_updated` field, document in schema spec | High | Low | Novak, Chen |
| 4 | Change template delimiter (not `${}`) | High | Low | Chen, Blackwell |
| 5 | `max_nodes_reached` flag in recall response | High | Low | Sharma |
| 6 | Structured warnings `{code, message, field}` | High | Low | Nakamura |
| 7 | Context-pack sort: weight then recency | High | Low | Vasquez |
| 8 | `vault status` cold-start command | Medium | Low | Nakamura |
| 9 | Fix `all-or-nothing` contradiction in 15-nonfunctional | Medium | Trivial | Blackwell |
| 10 | Add mget/apply response shapes to 09 | Medium | Low | Novak |

---

## Deferred to v2 Roadmap

- Agent access log table for recency tracking (Hoffmann, Vasquez)
- Salience/importance field in frontmatter (Hoffmann)
- Summary generated region + memory summarize command (Hoffmann)
- Semantic expansion extension point — define interface now (Hoffmann)
- Reflection note type (Hoffmann)
- Memory command consolidation (Nakamura) — revisit with usage data
- Per-node dirty flags vs global re-index (Sharma)

---

## Overall Assessment

**Session 01 fixes landed well.** The spec is materially stronger on graph correctness (BFS, cycle detection, indexes), agent contract (structured errors, TTY detection), and mutation safety (plan sequencing, honest atomicity).

**Session 02 revealed a deeper problem:** the agent's trust in its own memory is unfounded. Index staleness means post-write queries return lies. No write confirmation means retry is unsafe. No staleness signal means the agent can't self-correct. This cluster — staleness + no receipt + no dirty flag — is the primary failure mode for production agent use.

**The new experts added genuine value.** Nakamura's AX lens caught the cold-start tax, write_hash gap, and idempotency issues that a CLI/DX review would not. Hoffmann's AI memory framing (archival tier, missing working memory, retrieval signal gaps) recontextualizes VaultMind within the agent memory literature and provides a principled v2 roadmap.

**The spec is ready for implementation** once items 1-6 above are incorporated. Items 7-10 are important but not blocking. The v2 roadmap items (access logging, salience, embeddings) should be tracked but not designed until v1 usage data validates the need.
