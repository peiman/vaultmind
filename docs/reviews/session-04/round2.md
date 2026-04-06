# Expert Panel — Round 2: Cross-Review

**Date:** 2026-04-06 | **Reviewing:** Round 1 findings from all 7 experts

Each expert reviews the other experts' Round 1 findings, confirms or disputes them, and adds any missed observations.

---

## 1. Dr. Elena Vasquez — Cross-Review

### On Blackwell's BFS Edge Direction Bug (Expert 3)

**Confirmed — this is real and impacts retrieval interpretation.** The `TraverseEdge.SourceID` for inbound-discovered nodes reports the BFS parent, not the actual edge source. When `memory recall` builds `RecallEdge` from this data, the edge direction is inverted. An agent that reasons about link direction — "which notes cite this one?" or "this note is the authority because many notes link to it" — will draw the wrong conclusions.

However, I want to qualify the severity. For `memory recall`, the typical agent use case is "show me the neighborhood of this note." Most agents use the node list for context, not the edge directions for reasoning. The bug matters for graph-aware agents doing authority analysis or citation tracking, but not for basic context retrieval. **Rate as HIGH, not CRITICAL.**

### On Sharma's Missing Indexes (Expert 4)

**Confirmed.** The 1,500× improvement estimate at 10K notes is plausible. Adding `COLLATE NOCASE` indexes is the right fix — it avoids the `LOWER()` function call in the query, letting SQLite use the index directly.

### On Hoffmann's Retriever Interface (Expert 7)

**Agree in principle, disagree on timing.** The interface is architecturally correct but the cost of adding it now (when there is only one retrieval backend) is premature abstraction. The right time to extract the interface is when the second backend is being added — the interface should be shaped by two concrete implementations, not one.

---

## 2. Marcus Chen — Cross-Review

### On Vasquez's Tilde-Fence Gap in StripForAliasMatch (Expert 1)

**Confirmed — this is a real bug I should have caught.** The parser (`links.go:45`) correctly handles both ```` ``` ```` and `~~~` for link extraction. But `StripForAliasMatch` in `inferred.go:11` uses `codeFenceRe = regexp.MustCompile("(?s)` `` `` `` `[^` `` `]*` `` `` `` `")` which only matches triple-backtick fences.

A vault note with:
```
~~~python
The term "Machine Learning" appears here
~~~
```
...would produce a false `alias_mention` edge if "Machine Learning" is an alias in the vault. The fix is to extend `codeFenceRe` to match both fence types, consistent with the parser.

### On Blackwell's Tag Overlap Asymmetry (Expert 3)

**Confirmed and I agree this is a real issue.** In my vault, I frequently search for related notes from either direction. If note A and note B share tags, I expect `links out A` to show B and `links out B` to show A. The current implementation only shows one direction depending on which ID sorts lower.

### On Novak's OpenVaultDB Gap (Expert 5)

**Confirmed.** I tested this by pointing `--vault` at a non-existent path with `--json`. The output was raw text, not JSON. This is the most common misconfiguration an agent would encounter.

---

## 3. Jordan Blackwell — Cross-Review

### On Novak's SearchResult.Total (Expert 5)

**Confirmed — I raised this in Session 03 as Finding 15 and it is still not fixed.** `Total: len(results)` is wrong. The fix requires a separate COUNT query:

```sql
SELECT COUNT(*) FROM fts_notes f JOIN notes n ON n.id = f.note_id WHERE fts_notes MATCH ?
```

This is not a hard fix but it does require running an additional query. The performance cost is minimal since SQLite's FTS5 already computed the matching set.

### On Vasquez's Qualification of My BFS Bug Severity (Expert 1)

**Respectfully disagree.** The bug's severity depends on the consumer's use case, which we cannot predict. An agent that uses `memory recall` to build a citation graph, populate a backlinks panel, or reason about note authority will receive inverted edges. The output contract says `source_id` — that means the source of the edge, not the BFS parent. If we meant BFS parent, the field should be called `bfs_parent_id`.

The code already has the correct value available: `nb.id` for inbound edges IS the actual source (it was scanned from `src_note_id`). The fix is one line: swap `nb.sourceID = nodeID` to `nb.sourceID = nb.id` for the inbound case. A one-line fix with high semantic impact is the definition of a high-priority bug.

### On Sharma's Alias Map Collision (Expert 4)

**Good catch.** If aliases "API" and "api" both exist (pointing to different notes), `aliasToNoteID[strings.ToLower("API")]` overwrites `aliasToNoteID[strings.ToLower("api")]`. The last-scanned alias wins, which is non-deterministic (depends on SQL row order). This is a low-probability bug but a real one. Fix: use a slice instead of a map for multi-valued keys, or detect and skip collisions.

### On Hoffmann's Schema Migration (Expert 7)

**Confirmed.** Without a migration mechanism, v2 embedding support requires either:
- A manual `ALTER TABLE` that the user must run (bad UX)
- Dropping and recreating the database (loses index state)
- A migration table with version tracking (correct solution)

The migration mechanism should be added before v2 development starts, not during it.

---

## 4. Dr. Priya Sharma — Cross-Review

### On Blackwell's BFS Edge Direction Bug (Expert 3)

**Confirmed.** I want to add a specific example to illustrate:

Consider notes A→B (A's body contains `[[B]]`):
- BFS starts at B, depth 1
- Inbound query from B finds edge where `dst_note_id = B`, returns `src_note_id = A`
- Code: `nb.sourceID = nodeID` (B), `nb.id = A` (scanned)
- Output: `TraverseEdge{SourceID: "B", EdgeType: "explicit_link"}`
- `RecallEdge`: `{SourceID: "B", TargetID: "A", EdgeType: "explicit_link"}`

This says "B has an explicit_link to A." The truth is "A has an explicit_link to B." The edge direction and the identity of the linking note are both wrong.

### On Blackwell's Tag Overlap Asymmetry (Expert 3)

**Confirmed, and the fix has a subtlety.** Inserting bidirectional edges means the `idx_links_unique` constraint on `(src_note_id, dst_note_id, edge_type, dst_raw)` will need both (A,B) and (B,A) entries. The current `p.a < p.b` constraint prevents duplicates for unidirectional storage; for bidirectional, the code needs to insert two rows per qualifying pair.

However, this doubles the number of tag_overlap edges in the database. For a vault with 1000 qualifying pairs, that's 2000 rows instead of 1000. This is acceptable but worth noting for the performance budget.

### On Hoffmann's Missing Retriever Interface (Expert 7)

**Agree with Hoffmann, disagree with Vasquez.** The interface should be defined now, even with one backend. The cost is one type definition. The benefit is not just "clean architecture for future work" — it also makes the current code more testable. `SearchFTS` is called directly from `query/search.go`, which means tests must use a real SQLite database. An interface would allow mock-based testing of the retrieval layer.

---

## 5. Alex Novak — Cross-Review

### On Blackwell's BFS Edge Direction Bug (Expert 3)

**Confirmed.** I want to emphasize the agent impact: an MCP tool client that receives `{source_id: "B", target_id: "A", edge_type: "explicit_link"}` will interpret this as "B links to A." If B is "Project Plan" and A is "Meeting Notes," the agent thinks the project plan references the meeting notes. In reality, the meeting notes reference the project plan. This inverts the information hierarchy.

### On Nakamura's Missing Dry-Run for Note Create (Expert 6)

**Good catch.** The mutation engine has dry-run support (the `--dry-run` flag on frontmatter operations). Note create was added later (P3d) and does not have this flag. For an agent that validates before writing, this is a missing capability. Low effort to add.

### On Vasquez's Double QueryFullNote in Context-Pack (Expert 1)

**Confirmed as a performance issue.** In `contextpack.go`, each context item is loaded once during frontmatter packing (line 119) and again during body backfill (line 156). At 50 context items, that's 100 `QueryFullNote` calls instead of 50. Each call fires 5 sequential queries (per Session 03 Finding 12). So: 100 × 5 = 500 queries instead of 250. A simple cache (map[string]*FullNote) would halve the query count.

### On My Own OpenVaultDB Finding — Fix Approach

Having reviewed all 24 affected commands, I recommend a centralized fix. Add a `OpenVaultDBJSON` function to `cmdutil`:

```go
func OpenVaultDBJSON(cmd *cobra.Command, vaultPath, commandName string) (*VaultDB, error) {
    vdb, err := OpenVaultDB(vaultPath)
    if err != nil && isJSONOutput(cmd) {
        return nil, WriteJSONError(cmd.OutOrStdout(), commandName, "vault_error", err.Error())
    }
    return vdb, err
}
```

Each command replaces `OpenVaultDB(vaultPath)` with `OpenVaultDBJSON(cmd, vaultPath, "command-name")`. This is a mechanical change across 24 files.

---

## 6. Kai Nakamura — Cross-Review

### On Blackwell's BFS Edge Direction Bug (Expert 3)

**Confirmed.** From an AX perspective, this is particularly problematic because the output looks correct — it's valid JSON with the right field names. An agent has no way to detect that the source_id and target_id are swapped. Silent incorrect output is the worst category of agent-facing bug.

### On Novak's OpenVaultDB Fix Approach (Expert 5)

**Agree with the centralized fix.** I would add one enhancement: the error envelope should include the vault path in the error message. Currently, `"vault path X does not exist"` includes the path in the message string, but not as a structured field. Adding `Field: vaultPath` to the Issue struct would let agents programmatically extract and correct the path.

### On Hoffmann's Context-Pack Depth Limitation (Expert 7)

**Important observation.** The gap between `memory recall` (multi-hop) and `memory context-pack` (single-hop) means an agent that wants budget-constrained multi-hop context has no single-command path. Adding `--depth` to context-pack would be the most agent-useful enhancement for v2.

### Agent Error Recovery Matrix

Building on my Round 1 recommendation, here's the error recovery matrix an agent would need:

| Error Code | Recoverable? | Suggested Action |
|------------|-------------|-----------------|
| `vault_error` | Maybe | Verify vault path exists |
| `database_locked` | Yes | Retry after 1s (max 3 retries) |
| `not_found` | No | Try entity resolution first |
| `ambiguous` | Partial | Use candidates list to disambiguate |
| `path_traversal` | No | Validate path before retry |
| `empty_plan` | No | Check plan file content |

None of these suggested actions are surfaced in the current error output.

---

## 7. Dr. Lena Hoffmann — Cross-Review

### On Blackwell's BFS Edge Direction Bug (Expert 3)

**Confirmed.** The semantic impact extends beyond individual agent queries. If an agent is building a local knowledge graph from multiple `memory recall` calls, the inverted edges corrupt the graph's directed structure. Authority analysis (PageRank-style), citation chains, and influence tracking all depend on correct edge direction. This is a graph integrity bug, not just a display bug.

### On Vasquez's Disagreement About Retriever Interface Timing (Expert 1)

**Disagree with Vasquez.** The "extract interface when you have two implementations" heuristic applies when the interface shape is unknown. Here, the interface shape is known from the SRS — it's `func Search(query, opts) → []ScoredResult`. Defining it now costs one type and prevents the call-site coupling that makes the eventual extraction harder.

Additionally, defining the interface now would force the normalization question: the interface's return type would specify `Score float64` with documented semantics (0–1 range), which would require the BM25 implementation to normalize. This is a forcing function for a fix that should happen anyway.

### On Sharma's Alias Map Collision (Expert 4)

**Confirmed and worth generalizing.** The collision is a symptom of a broader issue: the alias/title namespace is flat. If two notes both have title "Introduction" (e.g., `ai-introduction` with title "Introduction" and `math-introduction` with title "Introduction"), the alias scanner can only map one. The correct fix is to make `aliasToNoteID` a `map[string][]string` (one alias → many note IDs) and insert edges for all matching notes, not just the last one.

### On Novak's Double QueryFullNote (Expert 5)

**Confirmed.** Beyond the performance impact, there's a correctness subtlety: if a note is modified between the frontmatter query and the body backfill query (e.g., by a concurrent mutation), the frontmatter and body could be from different versions. This is extremely unlikely in practice (context-pack runs in milliseconds) but architecturally, querying once and caching is both faster and more correct.

### Session 03 Findings — Summary of Resolution Status

Consolidating all experts' assessments:

| # | Session 03 Finding | Status | Notes |
|---|-------------------|--------|-------|
| 1 | Link resolution write-back | ✅ Resolved | 3-pass UPDATE in both Rebuild and Incremental |
| 2 | YAML date storage | ✅ Resolved | time.Time → YYYY-MM-DD format |
| 3 | Duplicate ID silent upsert | ⚠️ Partial | Warns but still overwrites |
| 4 | FTS input sanitization | ✅ Resolved | Quote-wrapping per word |
| 5 | FTS snippet column index | ✅ Resolved | Uses -1 (all columns) |
| 6 | Infrastructure errors bypass envelope | ⚠️ Partial | App errors fixed; OpenVaultDB errors still raw |
| 7 | SearchResult.Total page count | ❌ Unresolved | Still returns len(results), not total |
| 8 | JSON struct tags | ✅ Resolved | All response types tagged |
| 9 | Missing indexes (title, alias) | ❌ Unresolved | No index on notes(title) or aliases(alias) |
| 10 | meta.index_hash empty | ❌ Unresolved | Defined but never populated |
| 11 | Per-note transactions | ✅ Resolved | Batch transactions in Rebuild |
| 12 | QueryFullNote 5 queries | ❌ Unresolved | Now higher impact due to memory commands |
| 13 | Tilde fences | ⚠️ Partial | Fixed in parser, not in StripForAliasMatch |
| 14 | Missing indexes (COLLATE NOCASE) | ❌ Unresolved | Same as #9 |
| 15 | SearchResult.Total | ❌ Unresolved | Same as #7 |
| 16 | Exclude matching name-only | ❌ Unresolved | Documented as deferred |
| 17 | PascalCase field names | ✅ Resolved | Same as #8 |
| 18 | No Retriever interface / migration | ❌ Unresolved | Neither implemented |

**Resolved: 7/18 | Partial: 3/18 | Unresolved: 8/18**

The 3 critical blockers (items 1, 2, 5) are fully resolved. Most unresolved items are performance optimizations or architectural improvements that are not blocking for v1 but should be addressed before v2 development begins.
