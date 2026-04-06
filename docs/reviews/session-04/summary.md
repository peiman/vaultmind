# Session 04 — Summary of Findings

**Date:** 2026-04-06 | **Reviewing:** VaultMind v1 complete (Phase 1–3, ~37K lines, 1,807 tests, 85.5% coverage) | **Panel:** 7 experts

> Full transcripts: [Round 1](round1.md) | [Round 2](round2.md) | [Panel roster](panel.md)

---

## Implementation Assessment

VaultMind v1 is a substantial and well-engineered system. The jump from Phase 1 (~5,800 lines, 23 commands) to the complete v1 (~37,000 lines, 22 top-level commands + subcommands, 1,807 tests, 85.5% coverage) demonstrates consistent implementation discipline. All 23 quality checks pass. The Phase 2 and Phase 3 additions — mutation engine, incremental indexing, graph traversal, memory commands, plan execution, note creation — are architecturally clean and well-tested.

Of the 18 findings from Session 03, **7 are fully resolved** (including all 3 critical blockers), **3 are partially resolved**, and **8 remain unresolved** (mostly performance optimizations and architectural improvements appropriate for v2). The core data integrity bugs that blocked Session 03 are fixed.

The panel identified **1 new correctness bug**, **1 partially-addressed Session 03 bug with ongoing impact**, and several medium-priority improvements. No new blockers were found.

---

## Consensus Findings (4+ experts agree)

### 1. BFS INBOUND EDGE DIRECTION INVERTED — Correctness Bug
**Raised by:** Blackwell | **Confirmed by:** Sharma, Novak, Nakamura, Hoffmann, Vasquez
In `traverse.go:173-175`, inbound edge processing sets `nb.sourceID = nodeID` (the BFS parent) instead of the actual `src_note_id` from the database query. For an inbound edge A→B (where B is being expanded), the output reports `SourceID=B` when it should report `SourceID=A`. The edge direction is inverted for every node discovered through an inbound edge.

**Impact:** `memory recall` → `RecallEdge` reports wrong direction. `links neighbors` → `TraverseEdge` reports wrong direction. Any agent reasoning about link direction (citations, authority, backlinks) receives inverted information. The BFS traversal itself visits the correct nodes — this is an output contract bug, not a traversal bug.

**Fix:** For the inbound edge loop, set `nb.sourceID = nb.id` after scanning (the scanned `nb.id` is `src_note_id`, which is the actual edge source). One-line change.

**Panel disagreement on severity:** Vasquez rates HIGH (most agents use node lists, not edge directions). Blackwell rates CRITICAL (output contract says `source_id` means edge source, not BFS parent). **Consensus: HIGH — fix before any agent deployment.**

### 2. OpenVaultDB ERRORS BYPASS JSON ENVELOPE — AX Bug (24 commands)
**Raised by:** Novak | **Confirmed by:** Blackwell, Nakamura, Chen, Hoffmann
All 24 commands that support `--json` share the pattern:
```go
vdb, err := cmdutil.OpenVaultDB(vaultPath)
if err != nil { return err }  // raw text, not JSON
```
Three error classes bypass the envelope: vault path not found, config load failure, and database open failure (locked, permissions). An agent that sets `--json` and parses the response as JSON will crash.

**Fix:** Add `OpenVaultDBJSON(cmd, vaultPath, commandName)` wrapper in `cmdutil` that checks `--json` and calls `WriteJSONError` on failure. Mechanical change across 24 command files.

### 3. SearchResult.Total STILL RETURNS PAGE COUNT — Unresolved Session 03 Finding
**Raised by:** Blackwell (Session 03 Finding 15, re-confirmed) | **Confirmed by:** Novak, Nakamura, Sharma
`Total: len(results)` returns the number of hits on the current page (≤ limit), not the total number of matching documents. An agent displaying pagination information will show incorrect totals.

**Fix:** Add a separate `SELECT COUNT(*)` query against the FTS table with the same MATCH expression and filters. Return the count as `Total`.

### 4. MISSING DATABASE INDEXES ON title AND alias — Unresolved Session 03 Finding
**Raised by:** Sharma (Session 03 Finding 14, re-confirmed) | **Confirmed by:** Blackwell, Novak, Vasquez
No index on `notes(title)` or `aliases(alias)`. `ResolveLinks` performs case-insensitive lookups on these columns — every one is a full table scan. Sharma estimates a 1,500× improvement at 10K notes from adding `COLLATE NOCASE` indexes.

**Fix:** Add two statements to the schema:
```sql
CREATE INDEX IF NOT EXISTS idx_notes_title ON notes(title COLLATE NOCASE)
CREATE INDEX IF NOT EXISTS idx_aliases_alias ON aliases(alias COLLATE NOCASE)
```

### 5. meta.index_hash NEVER POPULATED — Unresolved Session 03 Finding
**Raised by:** Novak (Session 03 Finding 10, re-confirmed) | **Confirmed by:** Nakamura, Hoffmann, Blackwell
The `IndexHash` field is defined in the envelope Meta struct and the computation function exists, but no command ever sets it. The field ships as empty string in every response. Agents using this field for cache invalidation are permanently broken.

**Fix:** Either populate `IndexHash` in the envelope construction path, or remove the field entirely. An empty field that implies a capability is worse than an absent field.

### 6. TILDE-FENCE NOT STRIPPED IN ALIAS MENTION SCANNER
**Raised by:** Vasquez | **Confirmed by:** Chen, Blackwell, Sharma
`StripForAliasMatch` in `inferred.go:11` uses a regex that only matches triple-backtick fences, not tilde fences (`~~~`). The parser correctly handles both fence types. Aliases inside tilde-fenced code blocks produce false `alias_mention` edges.

**Fix:** Extend `codeFenceRe` to match both fence types: ``(?s)(```[^`]*```|~~~[^~]*~~~)`` or use a line-by-line fence tracker consistent with the parser.

---

## Strong Findings (3 experts, no opposition)

### 7. Duplicate ID Warns But Still Overwrites
**Raised by:** Blackwell | **Confirmed by:** Novak, Nakamura
Session 03 Finding 3 recommended "refuse the second insert." The current implementation logs a warning and increments a counter, but still `INSERT OR REPLACE`s the second file, destroying the first. Data loss still occurs — it's just logged now.

**Recommendation:** Either refuse the second insert and add the conflict to the response warnings, or explicitly document that warn-and-overwrite is the intended v1 behavior. The current state is ambiguous.

### 8. Tag Overlap Edges Are Unidirectional
**Raised by:** Blackwell | **Confirmed by:** Sharma, Novak
`ComputeTagOverlap` inserts edges with `p.a < p.b` (smaller ID → larger ID). The graph is asymmetric: `links out A` shows B, but `links out B` does not show A (only `links in B` does). `memory related` and `context-pack` query both directions and are unaffected.

**Recommendation:** Insert edges bidirectionally, or document the asymmetry. `links out` is the natural agent query for "what does this note connect to?" — it should show tag overlap neighbors regardless of ID ordering.

### 9. BM25 Scores Not Normalized to 0–1
**Raised by:** Hoffmann | **Confirmed by:** Novak, Vasquez
Scores are negated (positive = better) but not normalized. A score of 3.742 has no absolute meaning and cannot be blended with other signals. Normalization is prerequisite for multi-signal retrieval in v2.

### 10. No Retriever Interface or Schema Migration
**Raised by:** Hoffmann | **Confirmed by:** Sharma, Novak
`SearchFTS` is called directly with no interface abstraction. No schema migration mechanism exists. Both are prerequisites for v2 embedding support.

**Panel disagreement on timing:** Vasquez argues the interface should be extracted when a second implementation exists. Hoffmann and Sharma argue it should be defined now because the shape is known from the SRS and it improves testability. **Majority favors now.**

### 11. Context-Pack Queries DB Twice Per Context Item
**Raised by:** Vasquez | **Confirmed by:** Novak, Hoffmann
`contextpack.go` calls `QueryFullNote` during frontmatter packing (line 119) and again during body backfill (line 156) for the same note. At 50 context items, that's 500 queries instead of 250 (each QueryFullNote fires 5 sequential queries). A simple cache would halve the count.

### 12. Alias Map Collision for Normalized Duplicates
**Raised by:** Sharma | **Confirmed by:** Blackwell, Hoffmann
`aliasToNoteID` uses `strings.ToLower(text)` as key. Two aliases that normalize to the same lowercase string (e.g., "API" and "api" from different notes) collide; the last one wins non-deterministically. Fix: use `map[string][]string` and insert edges for all matching notes.

---

## Top 10 Action Items (Priority Order)

| # | Action | Severity | Effort | Source |
|---|--------|----------|--------|--------|
| 1 | Fix BFS inbound edge sourceID — swap `nb.sourceID` to `nb.id` for inbound case | **High** | Trivial | Blackwell, all |
| 2 | Route OpenVaultDB errors through JSON envelope (24 commands) | **High** | Low | Novak, all |
| 3 | Fix `SearchResult.Total` — add COUNT query for total doc count | **High** | Low | Blackwell, Novak |
| 4 | Add `CREATE INDEX` on `notes(title COLLATE NOCASE)` and `aliases(alias COLLATE NOCASE)` | **High** | Trivial | Sharma, all |
| 5 | Populate `meta.index_hash` or remove field from envelope | **Medium** | Low | Novak, Nakamura |
| 6 | Fix tilde-fence stripping in `StripForAliasMatch` | **Medium** | Trivial | Vasquez, Chen |
| 7 | Insert tag_overlap edges bidirectionally (or document asymmetry) | **Medium** | Low | Blackwell, Sharma |
| 8 | Fix alias map collision — use `[]string` for multi-valued keys | **Medium** | Low | Sharma, Hoffmann |
| 9 | Cache `QueryFullNote` results in context-pack | **Low** | Low | Vasquez, Novak |
| 10 | Normalize BM25 scores to 0–1 range | **Low** | Medium | Hoffmann, Novak |

---

## Session 03 Findings — Final Resolution Status

| # | Finding | Session 03 Severity | Session 04 Status |
|---|---------|-------------------|------------------|
| 1 | Link resolution write-back | Blocker | ✅ Resolved |
| 2 | YAML date storage corruption | Blocker | ✅ Resolved |
| 3 | Duplicate ID silent upsert | Blocker | ⚠️ Partial — warns but still overwrites |
| 4 | FTS input sanitization | High | ✅ Resolved |
| 5 | FTS snippet column index | High | ✅ Resolved |
| 6 | Infrastructure errors bypass envelope | High | ⚠️ Partial — OpenVaultDB errors still raw |
| 7 | SearchResult.Total page vs doc count | High | ❌ Unresolved |
| 8 | JSON struct tags on response types | High | ✅ Resolved |
| 9 | Missing indexes (title, alias) | Medium | ❌ Unresolved |
| 10 | meta.index_hash always empty | Medium | ❌ Unresolved |
| 11 | Per-note SQLite transactions | Performance | ✅ Resolved (batch transactions) |
| 12 | QueryFullNote 5 queries per note | Performance | ❌ Unresolved (higher impact now) |
| 13 | Tilde fences not detected | Medium | ⚠️ Partial — parser fixed, alias scanner not |
| 14 | Missing COLLATE NOCASE indexes | Medium | ❌ Same as #9 |
| 15 | SearchResult.Total | High | ❌ Same as #7 |
| 16 | Exclude matching name-only | Medium | ❌ Deferred (documented) |
| 17 | PascalCase field names | High | ✅ Same as #8 |
| 18 | No Retriever interface / migration | Medium | ❌ Unresolved |

**Summary: 7 resolved, 3 partial, 8 unresolved (of which 3 are performance, 2 are architectural, 3 are functional)**

---

## What Should Be Fixed Before Agent Deployment

These bugs affect the correctness or reliability of agent-facing output. An agent that consumes VaultMind's JSON output will encounter incorrect data or parsing failures without these fixes.

1. **Item 1 — BFS edge direction:** Agents receive inverted edge directions. One-line fix, high semantic impact.
2. **Item 2 — OpenVaultDB JSON envelope:** Agents crash on JSON parse when vault path is wrong. Mechanical 24-file fix.
3. **Item 3 — SearchResult.Total:** Agents display wrong pagination totals. Requires COUNT query addition.
4. **Item 4 — Missing indexes:** Index performance degrades at vault scale. Agents hitting large vaults will see timeouts. Trivial schema addition.

## What Should Be Fixed Before v2 Development

These are architectural prerequisites that should be in place before v2 features are built on top of them.

- **Item 5 — meta.index_hash:** Either populate or remove. Decide before v2 agents rely on it.
- **Item 10 — BM25 normalization:** Prerequisite for multi-signal retrieval.
- **Session 03 #18 — Retriever interface:** Define before a second retrieval backend is added.
- **Session 03 #18 — Schema migration:** Implement before embedding column is needed.

## What Can Wait

- **Item 6 — Tilde-fence alias stripping:** Affects only vaults using tilde fences with aliases inside them. Low probability.
- **Item 7 — Tag overlap bidirectional:** `memory related` and `context-pack` work correctly (they query both directions). Only `links out` is affected.
- **Item 8 — Alias map collision:** Affects only vaults with multiple aliases normalizing to the same lowercase string. Low probability.
- **Item 9 — Context-pack caching:** Performance optimization, not correctness.
- **Session 03 #3 — Duplicate ID overwrite:** Data loss is logged. Whether to refuse or overwrite is a product decision, not a bug.

---

## Overall Assessment

**VaultMind v1 is ready for agent deployment with 4 targeted fixes.** The 3 Session 03 critical blockers are resolved. The Phase 2 and Phase 3 additions are architecturally sound, well-tested, and deliver the core value proposition: associative retrieval across a knowledge graph, budget-constrained context packing, and a complete mutation pipeline.

The BFS edge direction bug (Item 1) is the only new correctness issue. It is a one-line fix. The OpenVaultDB envelope gap (Item 2) and SearchResult.Total (Item 3) are carry-overs from Session 03 with known fixes. The missing indexes (Item 4) are a trivial schema addition with outsized performance impact.

**The path from v1 to v2 is clear:** fix the 4 deployment-blocking items, add the Retriever interface and schema migration mechanism, then build embedding support and multi-signal retrieval on top of the clean foundation that v1 provides.
