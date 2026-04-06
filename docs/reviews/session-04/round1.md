# Expert Panel — Round 1: Independent Reviews

**Date:** 2026-04-06 | **Reviewing:** VaultMind v1 complete (Phase 1–3, ~37K lines, 1,807 tests, 85.5% coverage)

---

## Panel Members

| # | Expert | Specialty |
|---|--------|-----------|
| 1 | Dr. Elena Vasquez | Cognitive neuroscience, long-term memory |
| 2 | Marcus Chen | Obsidian power user, vault architecture |
| 3 | Jordan Blackwell | Devil's advocate, systems architecture |
| 4 | Dr. Priya Sharma | Knowledge graphs, graph databases |
| 5 | Alex Novak | AI agent systems engineering |
| 6 | Kai Nakamura | AX (AI Experience) design |
| 7 | Dr. Lena Hoffmann | AI/LLM memory systems research |

---

## 1. Dr. Elena Vasquez — Cognitive Neuroscience / Memory Review

### Session 03 Findings Status

- **Finding 4 (3/6 edge types missing):** `alias_mention` and `tag_overlap` are now implemented. `dataview_reference` remains absent (deferred to v2 per roadmap). The graph now has both structural and inferred edges. **Resolved.**
- **Finding 5 (no graph traversal wired to retrieval):** BFS traversal is implemented in `internal/graph/traverse.go` and wired to `memory recall` and `memory context-pack`. Associative retrieval is now functional end-to-end. **Resolved.**

### Memory Recall Assessment

The `memory recall` command implements what I described in Session 03 as the core value proposition: starting from a seed note, traversing the knowledge graph, and returning an enriched subgraph. The implementation is structurally sound:

- BFS with configurable depth, confidence thresholds, and node limits
- Enrichment with frontmatter, type, and title from the database
- Edge metadata preserved from the traversal

However, the recall output is **topology-only, not retrieval-ranked**. All nodes at the same BFS distance are treated equivalently. There is no scoring mechanism that combines graph proximity with recency, access frequency, or term relevance. An agent receiving 50 nodes at depth 2 has no way to prioritize among them without a blended score.

This is an expected v1 limitation — the graph traversal is correct, the ranking is Phase 2+/v2 work. But it should be explicitly documented as a current limitation, not just implicitly absent.

### Context-Pack Review

The `context-pack` command is the agent-facing retrieval surface. The priority ordering (explicit_relation > explicit_link > medium > low, with recency as tiebreaker) is a reasonable heuristic for cognitive importance:

- Explicit relations are the strongest signal — a human chose to create a typed link
- Inferred edges (alias mentions, tag overlap) are weaker but real signals
- Recency tiebreaker aligns with how human memory weighs more recent encounters

**Body backfill** is an important addition: after packing frontmatter for all context items, remaining budget is filled with note bodies in priority order. This is the right design — frontmatter gives breadth (many notes), body gives depth (fewer notes with full content).

**Concern:** The body backfill loop re-queries `QueryFullNote` for every context item, even though the same call was made during frontmatter packing (Step 6). This is a correctness-irrelevant performance issue at small scale but doubles database reads at the retrieval layer.

### Alias Mention Edge Quality

The `ComputeAliasMentions` implementation uses word-boundary regex matching after stripping code fences, inline code, wikilinks, and HTML comments. This is a reasonable first pass for medium-confidence inferred edges. However:

- **False negatives:** Common English words that happen to be note titles (e.g., "Focus", "Design") will match everywhere, producing noisy edges. The `minAliasLen` parameter provides some protection, but short titles like "AI" or "Go" would still be problematic if they exist in the vault.
- **Missing tilde-fence stripping:** `StripForAliasMatch` handles triple-backtick fences but not tilde-fenced code blocks (`~~~`). The parser correctly handles both fence types, but the alias mention scanner does not. Aliases inside tilde-fenced blocks will produce false edges.

### Recommendations

1. Document the topology-only limitation of `memory recall` — agents should know that node ordering within a distance tier is arbitrary.
2. Consider adding a `--min-alias-len` config option (currently hardcoded) to let vaults with short note titles raise the floor.
3. Fix tilde-fence handling in `StripForAliasMatch` to match the parser's behavior.

---

## 2. Marcus Chen — Obsidian Practitioner / Parser & Indexer Review

### Session 03 Findings Status

- **Finding 2 (YAML date corruption):** Fixed. `fmtString()` now detects `time.Time` values and formats as `2006-01-02` for date-only or RFC3339 for timestamps. **Resolved.**
- **Finding 13 (tilde fences not detected):** Fixed in the parser — `reCodeFence` in `links.go` now matches both ```` ``` ```` and `~~~`. **Resolved** for link extraction. **Not resolved** for alias mention detection (see Expert 1).
- **Finding 16 (exclude matching name-only):** No change. `.vaultmindignore` still matches on filename only. Path-based exclusion is not implemented. **Unresolved** but documented as deferred.

### Incremental Indexing

The incremental indexer (`internal/index/indexer.go`) is a significant Phase 2 addition:

- **Mtime fast path:** Files with unchanged mtime skip parsing entirely. This is the right default — most notes in a vault don't change between index runs.
- **Hash verification:** Files with changed mtime are hashed; if hash matches, only mtime is updated. This handles git checkout and editor touch operations correctly.
- **Deletion detection:** Set-difference between filesystem paths and indexed paths identifies orphaned entries.
- **Post-mutation re-index:** `IndexFile()` re-indexes a single file after mutations, keeping the index fresh without a full rebuild.

This is a well-designed incremental strategy. At 10K notes, the mtime fast path should reduce index time from minutes to seconds for typical "I edited 3 notes" runs.

### Template System

The `note create` template system uses `<%=var%>` substitution (not Go templates) to avoid collision with Obsidian Templater's `{{` syntax. This is a good design choice for the Obsidian ecosystem. The 8 supported variables (`id`, `type`, `title`, `created`, `updated`, `date`, `datetime`, `path`) cover the common frontmatter fields.

**ID generation** derives from filename: `{type}-{slug}`. This is sensible but could conflict with notes created outside VaultMind that use different ID schemes. The uniqueness check against the index catches this at creation time, which is the right place.

### Inferred Edge Quality Concerns

The alias mention scanner builds a single regex from all aliases and titles. At vault scale:

- **1,000 aliases → 1,000 regex alternatives.** The Go regex engine handles this but compilation time grows linearly. For very large vaults (10K+ notes, 30K+ aliases), the compile step could become the bottleneck.
- **Word-boundary matching is English-centric.** The `\b` word boundary in regex works well for Latin scripts but may miss or falsely match in CJK or other non-space-delimited writing systems.

### Recommendations

1. For large vaults, consider Aho-Corasick or a trie-based scanner instead of a single alternation regex.
2. Document the word-boundary limitation for non-Latin vault content.
3. Implement path-based exclude matching — this was a Session 03 strong finding and is still absent.

---

## 3. Jordan Blackwell — Devil's Advocate / Systems Review

### Session 03 Findings Status

- **Finding 3 (duplicate IDs silently upsert — BLOCKER):** Partially fixed. The indexer now detects duplicate IDs and logs a warning (`log.Warn`). However, the **second file still overwrites the first**. The Session 03 recommendation was to "refuse the second insert" — the current implementation warns but does not refuse. Data loss still occurs, it's just logged now. **Partially resolved** — the silence is fixed, the data loss is not.
- **Finding 6 (FTS input sanitization):** Fixed. `sanitizeFTSQuery` wraps each word in double quotes, preventing FTS5 operator interpretation. **Resolved.**
- **Finding 9 (infrastructure errors bypass envelope):** Partially fixed. Application-layer errors (e.g., note-not-found, path traversal) are now properly routed through the JSON envelope when `--json` is set. However, **infrastructure errors from `OpenVaultDB` still bypass the envelope on all 24 JSON-supporting commands.** "Vault not found", "database locked", and "permission denied" errors return raw text strings when `--json` is set. **Partially resolved** — same root cause as Session 03, narrower scope.
- **Finding 15 (`SearchResult.Total`):** Not fixed. `Total: len(results)` still returns the number of results on the current page, not the total matching document count. If a search matches 500 documents with `limit=20`, Total returns 20. **Unresolved.**

### BFS Traversal — Edge Direction Bug

**This is the most important new finding in this review.**

In `traverse.go:173-175`, the inbound edge handler sets `nb.sourceID = nodeID` before scanning the query result. For an inbound edge A→B (where B is the current node being expanded):

```go
// Query returns: src_note_id (A), edge_type, confidence, weight
nb.sourceID = nodeID  // sets to B
inRows.Scan(&nb.id, ...)  // nb.id = A (the src_note_id)
```

The resulting `TraverseEdge` reports `SourceID=B, TargetID=A`, meaning "B links to A." The actual database edge is A→B, meaning "A links to B." **The edge direction is inverted for every node discovered through an inbound edge.**

This is not a BFS bug — the traversal visits the correct nodes. It is an **output contract bug** that propagates into:

- `memory recall` → `RecallEdge.SourceID` is wrong for inbound-discovered nodes
- `links neighbors` → `TraverseEdge.SourceID` is wrong for inbound-discovered nodes
- Any agent reasoning about link direction (e.g., "which notes cite this one?") receives inverted information

**Fix:** For inbound edges, `nb.sourceID` should be set to `nb.id` (the actual `src_note_id` from the database), not `nodeID`. The BFS parent is `nodeID`, but `TraverseEdge.SourceID` should reflect the actual edge direction, not the BFS discovery path.

### Plan Executor Robustness

The plan executor (`internal/plan/executor.go`) has a good design: validate → execute → rollback on failure. But:

- **Rollback is in-memory only.** Pre-operation file bytes are stored in a byte buffer. If the process crashes mid-execution (OOM, SIGKILL), the rollback data is lost and files may be in an inconsistent state.
- **No execution log.** There is no persistent record of which operations were applied before the failure. An agent retrying a plan after a crash has no way to know which operations completed.

These are acceptable for v1 (plans are typically <20 operations on local files), but should be documented as limitations.

### Tag Overlap Edge Asymmetry

`ComputeTagOverlap` creates edges in one direction only: `INSERT (p.a, p.b, ...)` where `p.a < p.b` (smaller ID first). The graph is asymmetric for tag overlap edges.

`memory related` and `memory context-pack` query BOTH outbound and inbound edges, so they will find tag_overlap edges from both sides. However, `links out` only queries outbound edges from a given note. For note B where B > A:

- `links out B` → finds the edge (B has no outbound tag_overlap to A, only inbound)
- `links in B` → finds the edge (A→B direction)
- `links out A` → finds the edge (A→B direction) ✓
- `links in A` → does NOT find it (B→A direction doesn't exist)

This is a subtle asymmetry. An agent querying `links out` for two notes that share tags will see the edge from one note but not the other. **The fix is simple: insert edges in both directions.**

### Recommendations

1. **Fix the BFS inbound edge sourceID** — this is a correctness bug that affects all graph output.
2. **Fix `SearchResult.Total`** — this was a Session 03 finding and remains unresolved.
3. **Route `OpenVaultDB` errors through JSON envelope** — the remaining gap in Session 03 Finding 9.
4. **Decide on duplicate ID behavior** — either refuse the second insert (Session 03 recommendation) or document that warn-and-overwrite is the intended behavior. The current state is ambiguous.
5. **Insert tag_overlap edges bidirectionally** — or document the asymmetry.

---

## 4. Dr. Priya Sharma — Knowledge Graph Review

### Session 03 Findings Status

- **Finding 1 (link resolution write-back — BLOCKER):** Fixed. `ResolveLinks()` runs three UPDATE passes (ID, title, alias) after both `Rebuild()` and `Incremental()`. Wikilinks are now correctly resolved with `dst_note_id` and `resolved=TRUE`. **Resolved.**
- **Finding 7 (FTS snippet column index):** Fixed. Snippet function now uses column index `-1` (all indexed columns), not `1`. **Resolved.**
- **Finding 14 (missing indexes on title and alias):** Not fixed. The schema has `idx_aliases_normalized` on `aliases(alias_normalized)` and `idx_notes_type` on `notes(type)`, but there is **no index on `notes(title)` and no COLLATE NOCASE index on `aliases(alias)`**. `ResolveLinks` performs three UPDATE queries that do case-insensitive lookups on these columns — every one is a full table scan. **Unresolved.**

### Graph Completeness Assessment

The knowledge graph now contains 5 of 6 spec-defined edge types:

| Edge Type | Status | Confidence | Source |
|-----------|--------|------------|--------|
| `explicit_link` | Implemented | high | Parser: wikilinks |
| `explicit_embed` | Implemented | high | Parser: `![[embed]]` |
| `explicit_relation` | Implemented | high | Frontmatter: `relations` field |
| `alias_mention` | Implemented | medium | Body scan: `ComputeAliasMentions` |
| `tag_overlap` | Implemented | low | Tag co-occurrence: `ComputeTagOverlap` |
| `dataview_reference` | **Not implemented** | — | Deferred to v2 |

This is a substantial improvement from Session 03 where only 3 of 6 were present. The inferred edges (`alias_mention`, `tag_overlap`) add the associative signal that makes the graph useful for recall.

### Inferred Edge Algorithm Quality

**Alias Mentions:**

- **Correctness:** The algorithm strips markup, compiles a word-boundary regex from all aliases/titles, and inserts `alias_mention` edges with `confidence='medium'`. The deduplication via `edgeSet` prevents duplicate edges for the same source-destination pair. Self-referencing edges (note mentioning its own alias) are correctly excluded.
- **Concern — old edges cleared before recompute:** `DELETE FROM links WHERE edge_type = 'alias_mention'` runs before every computation. In incremental mode, this means ALL alias_mention edges are deleted and recomputed, even if only one note changed. At 10K notes with 5K alias_mention edges, this is wasteful.
- **Concern — case sensitivity in aliasToNoteID map:** The map key is `strings.ToLower(e.text)`, and matches are looked up via `strings.ToLower(match)`. This correctly handles case-insensitive matching. However, if two different aliases/titles normalize to the same lowercase string (e.g., "API" and "api"), the **last one wins** in the map. This is a subtle deduplication bug.

**Tag Overlap:**

- **Correctness:** The TF-IDF scoring is standard: `IDF = log(totalNotes / notesWithTag)`. Ubiquitous tags (IDF → 0) contribute nothing. Rare tags contribute heavily. The threshold parameter filters out weak associations.
- **Concern — `totalNotes` counts all domain notes, not just notes with tags.** A vault with 1000 domain notes where only 100 have tags will produce lower IDF scores than expected, because the denominator is 1000 not 100. This may suppress legitimate tag_overlap edges.

### BFS Traversal Graph Correctness

The BFS implementation is structurally correct — visited set prevents cycles, confidence filtering works, max-nodes is enforced. However, I concur with Expert 3's finding on the inbound edge sourceID issue. The `TraverseEdge.SourceID` for inbound-discovered nodes reports the BFS parent (the current node) rather than the actual edge source in the links table. This is semantically incorrect for any consumer that interprets `source_id` as the origin of a directed edge.

### Missing Indexes — Performance Impact

With Session 03 Finding 14 still open, I want to quantify the impact. `ResolveLinks` fires three UPDATE queries per index run:

1. `UPDATE links SET dst_note_id = (SELECT id FROM notes WHERE id = dst_raw)` — uses the UNIQUE index on `notes(id)`. ✓
2. `UPDATE links SET dst_note_id = (SELECT id FROM notes WHERE LOWER(title) = LOWER(?))` — **full table scan on notes**.
3. `UPDATE links SET dst_note_id = (SELECT note_id FROM aliases WHERE LOWER(alias) = LOWER(?))` — **full table scan on aliases**.

For a vault with 10K notes and 30K aliases, queries 2 and 3 scan 10K and 30K rows respectively, for every unresolved link after pass 1. At 5K unresolved links, that's 5K × 10K + 5K × 30K = 200M row scans. Adding `CREATE INDEX idx_notes_title ON notes(title COLLATE NOCASE)` and `CREATE INDEX idx_aliases_alias ON aliases(alias COLLATE NOCASE)` reduces this to 5K × log(10K) + 5K × log(30K) ≈ 135K lookups — a 1,500× improvement.

### Recommendations

1. **Add the missing indexes** — this is the highest-ROI change remaining. Trivial effort, enormous performance impact at scale.
2. Fix the alias-to-noteID map collision when two aliases normalize to the same lowercase string.
3. Consider incremental alias_mention computation instead of full delete-and-recompute.

---

## 5. Alex Novak — Agent Systems Review

### Session 03 Findings Status

- **Finding 8 (JSON struct tags):** Fixed. All major response types (`FTSResult`, `SearchResult`, `RecallNode`, `RecallEdge`, `ContextPackResult`, `RelatedItem`, `TraverseNode`, `TraverseEdge`, `Plan`, `Operation`, `TypeDef`) have `json:` struct tags. **Resolved.**
- **Finding 10 (`meta.index_hash` always empty):** Not fixed. The `IndexHash` field exists in `envelope.Meta` and the computation function exists in `cmdutil/helpers.go`, but **no command ever populates it**. Zero instances of `IndexHash` assignment found in any `cmd/*.go` file. The field ships as empty string in every JSON response. **Unresolved.**
- **Finding 17 (PascalCase field names):** Fixed (same as Finding 8). **Resolved.**
- **Finding 12 (`QueryFullNote` fires 5 queries):** No change. Still 5 sequential queries per note. This now has higher impact because `memory recall` and `memory context-pack` call `QueryFullNote` for every node in the result, multiplying the 5-query cost by the number of traversed nodes. **Unresolved** but acceptable for v1.

### Agent Output Contract — New Findings

**OpenVaultDB Error Envelope Gap (HIGH):**

All 24 commands that support `--json` share the same code pattern:

```go
vdb, err := cmdutil.OpenVaultDB(vaultPath)
if err != nil {
    return err  // raw text, even with --json set
}
```

Three error classes bypass the JSON envelope:
- `vault path "X" does not exist or is not a directory`
- `loading config: ...`
- `opening index: ...` (database locked, permission denied)

An agent that sets `--json` and parses the response as JSON will crash on these errors. This is the most common error path for any agent that misconfigures the vault path or encounters a locked database.

**Recommended fix:** Wrap `OpenVaultDB` errors in the JSON envelope. The simplest approach is a wrapper function in cmdutil that checks the `--json` flag and calls `WriteJSONError` on failure.

**SearchResult.Total Still Wrong (HIGH):**

`Total: len(results)` returns the page count (≤ limit), not the total matching document count. An agent displaying "showing 20 of 500 results" would display "showing 20 of 20 results." This was Session 03 Finding 15. It requires a separate `SELECT COUNT(*)` query against the FTS table with the same filters.

**BM25 Score Not Normalized (MEDIUM):**

Scores are negated (positive is better) but not normalized to 0–1. A score of `3.742` has no absolute meaning — it cannot be compared across queries or blended with other signals. This was noted in Session 03 Finding 10 as a prerequisite for multi-signal retrieval. The negation is an improvement but does not meet the spec's 0–1 range.

### Memory Commands — Agent Integration Assessment

The three memory commands are well-designed for agent use:

| Command | Input | Output | Agent Value |
|---------|-------|--------|-------------|
| `memory recall` | Note ID/title | Enriched subgraph | Explore neighborhood |
| `memory related` | Note ID/title | Direct edges with mode filter | Find specific edge types |
| `memory context-pack` | Note ID/title + budget | Token-budgeted context | Fill LLM context window |

`context-pack` is the most agent-useful command. The token budget parameter is exactly what an agent needs to manage context window limits. The priority ordering is sensible and the body backfill is a good addition.

**Missing: `context-pack` does not include the target note's links or tags in the budget.** An agent using context-pack as its primary retrieval surface gets frontmatter + body + related note frontmatter, but no explicit link/tag metadata for the target. This metadata is available via `note get --include-body`, but the agent has to make a second call and merge the results.

### Recommendations

1. **Fix OpenVaultDB JSON envelope gap** — affects all 24 commands. Single fix location.
2. **Fix SearchResult.Total** — requires a COUNT query but is a v1 contract bug.
3. **Populate meta.index_hash** — or remove the field. An empty field is worse than absent.
4. Normalize BM25 scores to 0–1 for future multi-signal blending.
5. Consider including target note links/tags in context-pack output.

---

## 6. Kai Nakamura — AX (AI Experience) Review

### Session 03 Findings Status

- **Finding 9 (infrastructure errors bypass envelope):** Partially fixed. Application-layer errors now use the envelope. **Infrastructure errors still bypass it** — same root cause identified by Expert 5. **Partially resolved.**
- **Finding related (stale binary in repo root):** The binary exists but is correctly `.gitignore`d and not tracked in git. **Resolved** (the file is a build artifact, not a committed file).
- **Finding related (`note mget` help visibility):** Confirmed visible in `note --help` output. **Resolved.**

### Agent Workflow: Cold Start to Retrieval

Testing the full agent workflow: vault discovery → indexing → search → recall → context-pack.

**Step 1: `vault status --json`** — Returns index state, note count, schema version. ✓ Good cold-start signal.

**Step 2: `index --json`** — Indexes the vault. Returns structured result with counts (added, updated, deleted, skipped, duplicate IDs). ✓ Good.

**Step 3: `search "topic" --json`** — Returns hits with snippets. The snippet now uses column `-1` (all columns), which is better than the Session 03 bug. Score is positive (negated BM25). ✓ Functional but Total is wrong (see Expert 5).

**Step 4: `memory recall <id> --json`** — Returns enriched subgraph with frontmatter. The output structure is clean and navigable. ✓ Good.

**Step 5: `memory context-pack <id> --json --budget 4000`** — Returns token-budgeted context. Priority ordering is clear. Body backfill fills remaining budget. ✓ Excellent agent surface.

**Workflow break: `--json` error handling.** If the vault path is wrong at any step, the agent receives raw text instead of JSON. This breaks the entire workflow. The agent cannot distinguish between "command not found" (shell error) and "vault not found" (VaultMind error) because neither is structured.

### Error Recoverability

For an agent to recover from errors, it needs:
1. A structured error code (to branch on)
2. A human-readable message (to log)
3. Optionally, a suggested action (to attempt recovery)

The envelope provides 1 and 2 when it's used. The gap is:
- **Infrastructure errors** (vault path, DB lock) don't use the envelope at all
- **No suggested actions** — an error like `database_locked` should suggest "retry after N seconds" or "close other VaultMind instances"

### New Feature Review: Note Create

`note create` is well-designed for the agent workflow:
- Template variables (`<%=id%>`, `<%=type%>`, etc.) avoid collision with Obsidian Templater
- Path traversal prevention is correct
- Required field validation is correct
- Post-create re-index keeps the index fresh

**Missing: no `--dry-run` flag on `note create`.** An agent that wants to validate inputs before writing has no way to check without side effects. The mutation engine has dry-run support for frontmatter operations, but note create does not.

### Hidden Flags

The Session 04 fix commit hides logging flags (`--log-level`, `--log-format`, etc.) from `--help` output. This is a good AX decision — agents don't need to see logging configuration in help output, but can still use the flags when explicitly needed.

### Recommendations

1. **Fix infrastructure error envelope gap** — the #1 agent-experience issue.
2. Add `suggested_action` field to the error envelope for common recoverable errors.
3. Add `--dry-run` to `note create` for validation-only workflows.
4. Document the full agent workflow (cold start → retrieval) in a single reference.

---

## 7. Dr. Lena Hoffmann — AI/LLM Memory Systems Review

### Session 03 Findings Status

- **Finding 10 (retrieval single-signal BM25, no blendable score):** Partially addressed. BM25 scores are now negated (positive = better), but **not normalized to 0–1**. A score of 3.742 cannot be blended with graph proximity (integer distance) or recency (date). The normalization prerequisite for multi-signal retrieval is not met. **Partially resolved.**
- **Finding 18 (no Retriever interface / no schema migration):** Not addressed. `SearchFTS` is still called directly with no interface abstraction. There is no schema migration mechanism. **Unresolved.**

### Memory Architecture Assessment

Phase 3 implements the three memory commands I recommended in Session 03. The architecture is clean:

```
Resolve(input) → Traverse(BFS) → Enrich(DB) → Pack(budget)
```

Each step is composable and independently testable. The separation between `graph/` (traversal) and `memory/` (enrichment + packing) is correct — the graph layer should not know about frontmatter or token budgets.

### Token Estimation

`EstimateTokens` uses `(len + 3) / 4` (ceiling division, ~4 chars per token). This is a reasonable heuristic for English text with Claude models. For non-English text (CJK, Arabic), token-per-character ratios differ significantly, but the heuristic is sufficient for v1 frontmatter packing where exact token counts are not critical.

### Context-Pack Depth Analysis

The context-pack implementation has a subtle design choice: it only considers **depth-1 edges** (direct neighbors), not the full BFS neighborhood. This means:

- For `memory recall --depth 3`, you get a 3-hop subgraph
- For `memory context-pack`, you get only direct neighbors regardless of any depth parameter

This is intentional (context-pack is budget-constrained, so shallower but richer is better than broader but thinner), but it means an agent that wants "the most important notes in a 3-hop neighborhood, packed into my budget" must combine `recall` + manual selection + `note mget` — there's no single-command path.

### Missing: Retrieval Frequency Tracking

Session 03 recommended adding `last_accessed_at` and `access_count` to the notes table as a prerequisite for recency-weighted retrieval. This is not implemented. Without access frequency data, the recency signal in context-pack uses the note's `updated` frontmatter field (when the note was last edited), not when it was last retrieved by an agent.

These are different signals:
- **Edit recency** (`updated`): "I last modified this note on March 15"
- **Access recency** (`last_accessed_at`): "An agent last asked about this note 2 hours ago"

For working-memory-like behavior (frequently accessed notes stay "hot"), access recency is the relevant signal. This is a v2 feature but the schema column should be planned.

### Missing: Retriever Interface

The absence of a `Retriever` interface means:
- `SearchFTS` is the only retrieval path, called directly in `query/search.go`
- Adding embedding-based retrieval requires forking this call site
- Adding graph-aware retrieval requires another fork
- Multi-signal blending has no clean composition point

The interface cost is one type definition. The benefit is a clean architecture for all future retrieval work. This should have been done in Phase 3 when the call graph was expanded with memory commands.

### Missing: Schema Migration

There is no migration mechanism. The schema uses `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`, which means:
- New tables/indexes can be added to existing databases ✓
- Column additions (e.g., `embedding BLOB`) require a manual `ALTER TABLE` with no upgrade path ✗
- Column type changes require dropping and recreating the table ✗

For v2 embedding support, a migration mechanism is prerequisite.

### Recommendations

1. **Define a `Retriever` interface** before v2 adds a second backend. This was recommended in Session 03 and is still the right call.
2. **Add a schema version table** with integer versioning and `up` migration scripts.
3. **Normalize BM25 scores to 0–1** as a prerequisite for multi-signal retrieval.
4. Plan the `access_count` / `last_accessed_at` schema addition for v2.
5. Consider a `context-pack --depth N` parameter for multi-hop budget-constrained packing.
