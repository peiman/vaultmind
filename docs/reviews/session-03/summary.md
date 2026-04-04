# Session 03 — Summary of Findings

**Date:** 2026-04-04 | **Reviewing:** Phase 1 implementation (Go, ~5,800 lines, 84.9% coverage) | **Panel:** 7 experts

> Full transcripts: [Round 1](round1.md) | [Round 2](round2.md) | [Panel roster](panel.md)

---

## Implementation Assessment

Phase 1 delivered the schema, scanner, parser, FTS search, entity resolver, and all CLI commands on spec. Test coverage (84.9%) and quality checks (23/23) are strong signals of implementation discipline. The schema matches SRS-12 exactly and the Go architecture is clean.

However, the panel identified **three correctness bugs that make the current build unsuitable for agent use** without fixes: link resolution write-back is absent (graph traversal is broken), YAML dates are stored as verbose Go `time.Time` strings (all date queries are broken), and duplicate IDs silently overwrite without warning (data is silently lost). These are not edge cases — they affect every vault on every index run.

---

## Consensus Findings (4+ experts agree)

### 1. LINK RESOLUTION WRITE-BACK MISSING — Phase 1 Correctness Blocker
**Raised by:** Sharma | **Confirmed by:** Blackwell, Novak, Nakamura, Vasquez
Body wikilinks are parsed and stored with `resolved=false, dst_note_id=NULL`. After entity resolution runs, the resolved `dst_note_id` is never written back to the `links` table. Wikilinks remain permanently unresolved. Consequences: `LinksIn` returns empty results for wikilink targets; graph traversal from any wikilink source or target is broken; the `resolved` field in all `links out` responses is a lie.
**Fix:** After entity resolution, write back `dst_note_id` for all successfully resolved links. This is prerequisite to any graph traversal work.

### 2. YAML DATE STORAGE CORRUPTION — Phase 1 Correctness Blocker
**Raised by:** Chen | **Confirmed by:** Novak, Nakamura, Vasquez, Hoffmann
`yaml.v3` parses YAML date scalars as `time.Time`. The `fmtString` fallback serializes these as `"2026-04-03 00:00:00 +0000 UTC"` instead of `"2026-04-03"`. Every `created` and `updated` field in every indexed note is stored in this verbose format. All date-based queries, range filters, and sort operations are broken. This is a silent data-corruption bug — there is no error signal.
**Fix:** Detect `time.Time` values in the YAML marshaling path and format them as `YYYY-MM-DD` (or `YYYY-MM-DDTHH:MM:SSZ` for timestamps that carry sub-day precision).

### 3. DUPLICATE IDs SILENTLY UPSERT — Data Loss Without Warning
**Raised by:** Blackwell | **Confirmed by:** Novak, Nakamura, Chen, Sharma
`INSERT OR REPLACE` silently overwrites an existing note when a second file carries the same frontmatter `id`. No conflict error is raised. No warning appears in the response envelope. The overwritten note's data is permanently gone with no trace in the index or the log.
**Fix:** Detect ID conflicts at insert time. Surface a structured warning (`{code: "DUPLICATE_ID", message: "...", paths: [file_a, file_b]}`) and refuse the second insert, or halt the index run. Never silently overwrite.

### 4. THREE OF SIX EDGE TYPES NOT IMPLEMENTED
**Raised by:** Vasquez | **Confirmed by:** Sharma, Blackwell, Novak, Hoffmann
`alias_mention` (medium confidence), `tag_overlap` (low confidence), and `dataview_reference` are absent. The graph contains only structural edges (`explicit_link`, `explicit_embed`, `explicit_relation`). All inferred edges — which carry the associative signal that makes the graph useful for recall — are missing. This is a planned Phase 2 gap but it means the retrieval value proposition is not yet delivered.

### 5. NO GRAPH TRAVERSAL WIRED TO RETRIEVAL
**Raised by:** Vasquez | **Confirmed by:** Sharma, Novak, Hoffmann, Nakamura
`memory recall` and `context-pack` do not traverse the stored graph. The graph is built and stored correctly but is not read during retrieval. BFS traversal is unimplemented. The core value proposition of VaultMind — associative retrieval across a knowledge graph — is not yet functional end-to-end.

### 6. FTS INPUT NOT SANITIZED
**Raised by:** Blackwell | **Confirmed by:** Novak, Nakamura, Sharma
Raw user input is passed directly to the SQLite FTS5 `MATCH` expression. FTS5 special characters (`"`, `*`, `(`, `)`, the keywords `AND`, `OR`, `NOT`, `NEAR`) cause SQLite errors that surface as unhandled panics or raw error strings. This is both a stability bug and, in MCP context, a potential injection vector.
**Fix:** Sanitize or quote all user-supplied FTS query strings before they reach the `MATCH` expression.

### 7. FTS SNIPPET COLUMN INDEX WRONG
**Raised by:** Sharma | **Confirmed by:** Nakamura, Novak, Chen
The FTS snippet call passes column index `1`, which maps to `title`. The body content is at column `0`. Search result snippets show the note title (a few words) instead of the matched body passage. This makes snippet output useless for triage.
**Fix:** Change snippet column index from `1` to `0`.

### 8. `meta.index_hash` ALWAYS EMPTY
**Raised by:** Novak | **Confirmed by:** Nakamura, Hoffmann, Blackwell
The `index_hash` field is present in every response envelope but is never populated. Any agent or tooling that uses this field for cache invalidation (the documented use case) is permanently broken. The field signals a capability it does not have.
**Fix:** Either populate the field (compute a hash of the vault file tree or the SQLite WAL checkpoint) or remove it from the response schema. An empty field is worse than an absent one.

### 9. INFRASTRUCTURE ERRORS BYPASS JSON ENVELOPE
**Raised by:** Nakamura | **Confirmed by:** Novak, Blackwell, Hoffmann
Errors like "vault not found", "database locked", and "permission denied" are returned as plain text outside the JSON envelope. Agents that parse the `status` field to detect failures silently swallow these errors. There is no structured `{code, message}` object for the agent to act on.
**Fix:** All error paths must return a valid JSON envelope with `status: "error"` and a structured error object. No raw string responses on any code path.

### 10. RETRIEVAL IS SINGLE-SIGNAL (BM25 ONLY), NO BLENDABLE SCORE
**Raised by:** Hoffmann | **Confirmed by:** Vasquez, Novak, Nakamura, Chen
Search returns raw negative BM25 floats. There is no recency signal, no graph-proximity signal, and no importance signal. The score cannot be blended with other signals because it is not normalized. This is an architectural constraint, not a missing feature — the current call graph has no `Retriever` abstraction and no normalized score contract.
**Fix (Phase 2):** Normalize BM25 scores to 0–1. Define a `Retriever` interface. Both are prerequisite to any multi-signal retrieval work.

---

## Strong Findings (3–4 experts, no opposition)

### 11. Per-Note SQLite Transactions — Performance Bottleneck at Scale
**Raised by:** Chen | **Confirmed by:** Blackwell, Nakamura
One transaction per note = one SQLite commit per note. At 5,000 notes this produces 5,000 round-trips. Benchmark data suggests 3–5× speedup from batching at 500–1,000 notes per transaction.

### 12. `QueryFullNote` Fires 5 Sequential Queries Per Note
**Raised by:** Blackwell | **Confirmed by:** Novak, Nakamura
Frontmatter, body, tags, links, and aliases are fetched in five separate round-trips per note. A single JOIN-based query would serve all fields in one pass. At bulk-read scale, this multiplies latency by 5.

### 13. Tilde-Fence Code Blocks Not Detected
**Raised by:** Chen | **Confirmed by:** Sharma, Blackwell
The parser detects triple-backtick fences but not tilde-style fences (`~~~`). Wikilinks inside tilde-fenced blocks are extracted as real links, producing false edges in the graph.

### 14. Missing Indexes on `notes(title)` and `aliases(alias)`
**Raised by:** Sharma | **Confirmed by:** Blackwell, Novak
Entity resolution — the most frequent read operation — does case-insensitive lookups on these columns. Without indexes, every resolution is a full table scan. At 10K notes, this becomes the dominant query cost.

### 15. `SearchResult.Total` Returns Page Count, Not Document Count
**Raised by:** Blackwell | **Confirmed by:** Novak, Nakamura
The `Total` field in search responses contains the page count, not the total number of matching documents. Every display of "N results found" is incorrect. This is a client-visible contract bug.

### 16. Exclude Matching Is Name-Only, Not Path-Based
**Raised by:** Chen | **Confirmed by:** Blackwell, Nakamura
`.vaultmindignore` patterns match on filename only. Directory subtree exclusions (e.g., `archive/`) do not work. Vaults with structured layouts cannot properly exclude directories.

### 17. `schema list-types` Returns PascalCase Field Names (No JSON Tags)
**Raised by:** Novak | **Confirmed by:** Nakamura, Hoffmann
`TypeDef` and related structs have no `json:` struct tags. Field names serialize as `Name`, `Description`, `Fields` — not `name`, `description`, `fields`. Agent code written against the spec will fail to deserialize.

### 18. No `Retriever` Interface / No Schema Migration Path
**Raised by:** Hoffmann | **Confirmed by:** Novak, Nakamura
`SearchFTS` is called directly with no interface abstraction. Adding a second retrieval backend requires modifying every call site. There is also no schema migration mechanism; adding an `embedding` column in Phase 2 requires a manual schema change with no upgrade path.

---

## Top 10 Action Items (Priority Order)

| # | Action | Severity | Effort | Source |
|---|--------|----------|--------|--------|
| 1 | Fix link resolution write-back — store `dst_note_id` after entity resolution | **Blocker** | Medium | Sharma, Blackwell, Novak |
| 2 | Fix YAML date storage — format `time.Time` as `YYYY-MM-DD`, not Go default string | **Blocker** | Low | Chen, Novak, Nakamura |
| 3 | Fix duplicate-ID upsert — detect conflict, emit structured warning, refuse second insert | **Blocker** | Low | Blackwell, Novak |
| 4 | Sanitize FTS user input before MATCH expression | **High** | Low | Blackwell, Novak |
| 5 | Fix FTS snippet column index (1 → 0 for body content) | **High** | Trivial | Sharma, Nakamura |
| 6 | Route all infrastructure errors through JSON envelope | **High** | Low | Nakamura, Novak |
| 7 | Fix `SearchResult.Total` — return document count, not page count | **High** | Trivial | Blackwell, Novak |
| 8 | Add `json` struct tags to all response types (`TypeDef` and peers) | **High** | Low | Novak, Nakamura |
| 9 | Add `CREATE INDEX` on `notes(title COLLATE NOCASE)` and `aliases(alias COLLATE NOCASE)` | **Medium** | Trivial | Sharma, Blackwell |
| 10 | Populate or remove `meta.index_hash` — no empty fields in the public contract | **Medium** | Low | Novak, Nakamura |

---

## What Blocks Phase 2 vs What Can Wait

### Phase 2 Blockers (must fix before building on top of the index)

These bugs mean the data in the index cannot be trusted. Any Phase 2 feature built on the current state will inherit corrupted or incomplete data.

- **Item 1 — Link resolution write-back:** Graph traversal, `memory recall`, `LinksIn`, and `context-pack` are all broken without this. `alias_mention` and `tag_overlap` edges (the primary Phase 2 deliverables) depend on resolved link data.
- **Item 2 — YAML date storage:** Every Phase 2 feature that filters, sorts, or displays dates will produce wrong results. Recency-weighted retrieval (a Phase 2 goal) is impossible with corrupted date fields.
- **Item 3 — Duplicate ID silent upsert:** Phase 2 indexing runs on real vaults will silently destroy data. Incremental re-index (a Phase 2 goal) makes this worse — every re-index run is a new opportunity for a silent overwrite.
- **Item 6 — Infrastructure errors bypass envelope:** Phase 2 agent integrations will silently fail on vault-not-found and DB-locked errors. No structured error = no recovery path.

### Fix Before Shipping (not blockers but high visible impact)

- **Item 4 — FTS sanitization:** Any agent passing user-supplied queries will encounter crashes. Fix before any public exposure.
- **Item 5 — FTS snippet column:** Search result snippets are broken today. Fix is one line.
- **Item 7 — `SearchResult.Total`:** Every search-based UI or agent displaying a result count is wrong.
- **Item 8 — JSON struct tags:** The `schema list-types` command is unusable by agents until fixed.

### Phase 2 Work (planned gaps, not bugs)

- Implement `alias_mention` edge type (highest retrieval signal — do first)
- Wire BFS graph traversal to `memory recall` and `context-pack`
- Implement `tag_overlap` edge type
- Define `Retriever` interface and normalize BM25 scores to 0–1
- Add schema migration mechanism (prerequisite for embedding column)
- Batch SQLite transactions for indexer performance
- Add `CREATE INDEX` items (Item 9 — low effort, high payoff at scale)
- Add `--max-body-chars` to `note get`
- Implement path-based exclude matching
- Add tilde-fence detection to parser
- Track `last_accessed_at` and `access_count` on notes (prerequisite for recency-weighted retrieval)

---

## Deferred to Phase 3 / v2 Roadmap

- Hybrid retrieval: BM25 score blended with graph proximity and recency
- Embedding-based semantic expansion extension point (define interface in Phase 2, implement in Phase 3)
- Agent access log table (`agent_access_log`) for frequency and recency tracking
- Importance / salience scoring at write time
- `memory summarize` command and `vm:summary` generated region
- Reflection note type
- `dataview_reference` edge type

---

## Overall Assessment

**Phase 1 is structurally sound but not yet correct.** The schema, CLI architecture, entity resolver, and test coverage are all strong. The implementation discipline is evident. But three correctness bugs — absent link resolution write-back, corrupted date storage, and silent duplicate-ID overwrites — mean the index cannot be trusted as a data source. No Phase 2 feature should be built on top of the current index without these three fixes in place.

**The retrieval gap is expected, not a failure.** Phase 1 was scoped to infrastructure: schema, scanner, parser, FTS, entity resolution. Phase 2 will wire graph traversal to retrieval and implement the inferred edge types. The panel's consensus is that this is the right sequencing — build the graph correctly before building retrieval on top of it.

**Five findings require only trivial or low effort** (Items 5, 7, 8, 9, and partial Item 10) and should be addressed immediately as they unblock agent testing and provide high confidence returns on minimal investment.

**The path to a trustworthy Phase 2 is clear:** fix the three blockers, ship the trivial fixes, then implement `alias_mention` + BFS traversal as the first Phase 2 milestone.
