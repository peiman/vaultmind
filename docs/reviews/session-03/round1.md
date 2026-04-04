# Expert Panel — Round 1: Independent Reviews

**Date:** 2026-04-04 | **Reviewing:** VaultMind Phase 1 implementation (Go, ~5,800 lines, 84.9% coverage)

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

### Retrieval Architecture

- **FTS uses BM25 — lexical only, no graph awareness.** The search layer operates purely on term frequency. There is no path from search results into the graph; a note that is highly connected to a seed but shares no vocabulary is invisible to BM25. Graph proximity and lexical relevance are computed in entirely separate pipelines with no blending mechanism.
- **Entity resolver cascade is well-aligned with cognitive recall.** The five-tier cascade (exact ID → title normalized → alias normalized → substring → ambiguous) is a reasonable computational model of how human recall degrades from precise to approximate. This is the strongest part of the implementation from a memory-science standpoint.

### Missing Edge Types

Three of the six spec-defined edge types are absent from the implementation:

- **`alias_mention` (medium confidence)** — not implemented. This is the most associatively rich inferred edge type: a note that mentions a concept by name is asserting a semantic relationship. Its absence is the largest gap for retrieval quality.
- **`tag_overlap` (low confidence)** — not implemented. Tag co-occurrence is a weak but real signal; its absence means the graph has no shared-taxonomy edges.
- **`dataview_reference`** — not implemented. Lower priority given the low agent relevance of Dataview queries, but still a spec gap.

### Memory Recall and Traversal

- **No memory recall / context-pack traversal.** The graph has `explicit_link` and `explicit_embed` edges but no path traversal is wired to the retrieval layer. `memory recall` and `context-pack` do not traverse the stored graph. Associative retrieval — the core value proposition of the system — is not yet functional.

### Recommended Phase Priority

1. `alias_mention` edge type first — highest retrieval signal, medium confidence
2. BFS graph traversal wired to `memory recall` / `context-pack`
3. `tag_overlap` edge type
4. Hybrid retrieval: BM25 score blended with graph proximity

---

## 2. Marcus Chen — Obsidian Practitioner / Parser Review

### Parser Bugs

- **Bug 1: Tilde fences (`~~~`) not detected.** The parser recognizes triple-backtick code fences but not tilde-style fences. Wikilinks inside `~~~` blocks are extracted as real links. This produces false edges in the graph for any vault that uses tilde-fenced code blocks.
- **Bug 2: Templater `{{date:YYYY-MM-DD}}` crashes YAML parsing.** Templater date expressions in frontmatter are not sanitized before YAML parsing. The `{{` token causes the YAML parser to fail, aborting indexing for that note.
- **Bug 3 (HIGH — data corruption): YAML dates stored as verbose Go time.Time string.** `yaml.v3` parses YAML date values (e.g., `2026-04-03`) as `time.Time`. The `fmtString` fallback then produces `"2026-04-03 00:00:00 +0000 UTC"` instead of `"2026-04-03"`. This affects every `created` and `updated` field in every note. All date-based queries against these fields are broken. This is a silent data-corruption bug, not a display issue.

### Performance

- **Per-note SQLite transactions will be a bottleneck at 5,000+ notes.** The indexer opens and commits one transaction per note. At 5K notes this produces 5K round-trips. Batching in chunks of 500–1,000 notes per transaction would reduce index time by an order of magnitude.

### Path Matching

- **Exclude matching is name-only, not path-based.** The `.vaultmindignore` exclusion logic matches on filename only. A pattern like `archive/` intended to exclude a directory subtree does not work — only exact filename matches are honored. Vaults with structured directory layouts cannot properly exclude subdirectories.

### Recommendations

1. **Fix Bug 3 immediately** — it corrupts the stored representation of every date field in every indexed note.
2. Add tilde-fence detection to the parser alongside backtick-fence detection.
3. Strip or escape Templater expressions before YAML parsing.
4. Batch SQLite inserts — commit every 500–1,000 notes, not every 1.
5. Implement glob/path-based exclude matching.

---

## 3. Jordan Blackwell — Devil's Advocate / Systems Review

### Atomicity and Safety

- **Rebuild is not atomic — process kill leaves a half-built index.** The index rebuild overwrites the live database in place. A killed process (OOM, SIGKILL, power loss) leaves the index in an inconsistent state with no recovery path and no signal to the operator.
- **Scanner aborts entirely on one permission error.** A single unreadable file causes the entire index run to fail. Partial indexing with a warning would be far more useful; hard abort is the wrong default for a tool operating on user-owned vaults.

### Input Sanitization

- **FTS MATCH receives raw user input with zero sanitization.** Special characters in FTS5 syntax (`"`, `*`, `(`, `)`, `AND`, `OR`, `NOT`) are passed directly to the SQLite FTS engine. A query like `note*` or `(crash` raises a SQLite error that surfaces to the caller as an unhandled exception. This is both a stability bug and a potential injection vector.

### Performance

- **Per-note transactions kill performance at scale** (confirmed with Marcus Chen — separate root causes but same fix applies).
- **`QueryFullNote` fires 5 separate queries per note** — frontmatter, body, tags, links, aliases are fetched in 5 sequential round-trips. A single JOIN-based query would serve all fields in one pass.
- **`Validate` issues N serial subqueries** — one per note, not a single set-based query. At vault scale, validation time is O(N) database calls.

### Correctness

- **MOST DANGEROUS: duplicate IDs silently upsert — second file overwrites first with no warning.** The upsert logic uses `INSERT OR REPLACE`. If two files carry the same frontmatter `id`, the second silently replaces the first. No conflict error, no warning in the response envelope, no operator signal. Data is lost without trace.
- **`SearchResult.Total` is page count, not total count.** The `Total` field in search responses returns the number of pages, not the total number of matching documents. Every caller computing "N results found" is displaying a lie.

### Recommendations

1. Fix the duplicate-ID upsert — detect the conflict and surface it as a warning with both file paths.
2. Fix `SearchResult.Total` — this is a client-visible correctness bug in every search response.
3. Add FTS input sanitization before any MATCH call.
4. Use atomic write (write to temp file, rename) for index rebuild.
5. Continue scanning on permission errors; collect them in a `warnings` array.

---

## 4. Dr. Priya Sharma — Knowledge Graph Review

### Schema Compliance

- **Schema matches SRS-12 exactly.** The 8-table DDL, all column names, all constraint names, and the FTS virtual table definition are correct. This is the cleanest part of the implementation.

### Missing Edge Types

- **Only 3 of 6 spec edge types are implemented:** `explicit_link`, `explicit_embed`, `explicit_relation`. Missing: `alias_mention`, `tag_overlap`, `dataview_reference`. The graph is structurally correct but semantically incomplete — all inferred edges are absent.

### Missing Indexes

- **No index on `notes(title)`.** Case-insensitive title lookups (the most common entity resolution path) do full table scans.
- **No index on `aliases(alias)`.** Alias resolution does full table scans on every lookup.

### FTS Snippet Bug

- **FTS snippet column index may be wrong.** The snippet call passes column index `1`, which points to `title`. The body content is column `0` in the FTS virtual table definition. Search snippets are being generated from the title column rather than the body, producing unhelpful one-word excerpts.

### Link Resolution Write-Back

- **CRITICAL: No link resolution write-back.** Body wikilinks are parsed and stored with `resolved=false, dst_note_id=NULL`. After entity resolution runs and resolves these links, the resolved `dst_note_id` is never written back to the `links` table. Wikilinks remain permanently unresolved in the database. This is not a performance issue — it is a correctness failure that makes the graph unusable for traversal.
- **`LinksIn` only works for `explicit_relation` edges, not body wikilinks.** Because wikilinks are never resolved, the `LinksIn` query filters on `dst_note_id` and finds nothing for wikilink targets. Inbound link counts and traversal from target to source are broken for the entire wikilink edge class.

### Recommendations

1. **Fix link resolution write-back** — this is a Phase 2 blocker. Graph traversal is impossible without it.
2. Add `CREATE INDEX idx_notes_title ON notes(title COLLATE NOCASE)`.
3. Add `CREATE INDEX idx_aliases_alias ON aliases(alias COLLATE NOCASE)`.
4. Fix FTS snippet column index (swap 0 and 1).
5. Implement `alias_mention` and `tag_overlap` edge types for Phase 2.

---

## 5. Alex Novak — Agent Systems Review

### Output Contract Bugs

- **Search scores are raw negative BM25 floats, not normalized 0–1.** Scores like `-3.742` are opaque to agents. The spec documents a 0–1 score range. The current output breaks any agent logic that thresholds on score.
- **`schema list-types` returns PascalCase field names with no JSON tags.** The `TypeDef` struct has no `json:` struct tags. Field names serialize as `Name`, `Description`, `Fields` — not `name`, `description`, `fields`. Agent code written against the spec will fail to deserialize.
- **`meta.index_hash` is always empty.** The field is present in the envelope but never populated. Cache invalidation logic that depends on this field is permanently broken.

### Date Storage

- **`created` and `updated` stored as Go `time.Time` string** (confirmed with Marcus Chen — same root cause, different manifestation). Agents parsing date fields receive `"2026-04-03 00:00:00 +0000 UTC"` instead of `"2026-04-03"`.

### Link Resolution

- **`links out` shows `resolved=false` for IDs that exist in the vault.** Because link resolution write-back is not implemented (see Expert 4), outbound links that were successfully resolved during indexing are still returned with `resolved=false`. Agents cannot trust the `resolved` field.

### Discoverability

- **`note mget` may be unreachable from `note --help`.** The subcommand is registered but does not appear in the help output for the `note` group. Agents performing help-based discovery will not find it.

### Recommendations

1. Normalize BM25 scores to 0–1 range before serialization.
2. Add `json` struct tags to all response types, starting with `TypeDef`.
3. Populate `meta.index_hash` — or remove the field to avoid false-confidence.
4. Fix `note mget` registration in help output.
5. These are all output-contract bugs visible to every agent caller — treat as Phase 1 blockers.

---

## 6. Kai Nakamura — AX (AI Experience) Review

### Cold-Start and Discovery

- **Cold-start via `vault status` works well.** The `vault status` command introduced in Session 02 is implemented and returns index state, note count, and schema version in one call. This is a genuine usability win.
- **`meta.index_hash` always empty** — confirmed by Alex Novak. Cache invalidation is broken. An agent that uses `index_hash` to decide whether to re-query will always receive a stale or absent hash and cannot make a correct decision.

### Note Access

- **`note get` has no body projection / truncation.** The `--include-body` flag is implemented but there is no `--max-body-chars` or `--body-lines` parameter. A note with a 50KB body returns the entire content. Context-window exhaustion is one `note get` call away on large notes.
- **Search snippets return title, not body excerpts** — confirmed by Expert 4 (FTS column index bug). The snippet shown in search results is the note title repeated, not the matched passage from the body. This makes search results useless for agent triage.

### Error Envelope

- **Infrastructure errors bypass the envelope and return raw strings.** Errors like "vault not found" and "database locked" are returned as plain text outside the JSON envelope. Agents that parse the `status` field of the envelope to detect errors will silently swallow these failures and receive no structured error to act on.
- **Unresolved links surfaced without guidance.** Search and recall results include links with `resolved=false` and `dst_note_id=null` but provide no `hint` or `suggested_action` field to tell the agent what to do.

### Housekeeping

- **Stale binary in repo root.** A compiled `vaultmind` binary has been committed to the repository root. This will diverge from source immediately and mislead any user who runs it instead of building from source.

### Recommendations

1. Route all infrastructure errors through the JSON envelope — no raw string responses.
2. Fix search snippet column index (same fix as Expert 4).
3. Add `--max-body-chars` to `note get`.
4. Remove stale binary from repo root; add to `.gitignore`.
5. Populate `meta.index_hash` or remove it.

---

## 7. Dr. Lena Hoffmann — AI/LLM Memory Systems Review

### Tier Assessment

- **VaultMind is archival-tier only (no working memory).** This was noted in Session 02 and remains true of the implementation. The system stores and indexes notes but has no session-scoped working memory concept. Agents must manage working memory externally, which is the expected v1 scope.

### Retrieval Signal

- **Retrieval is single-signal: BM25 only.** There is no recency weighting, no importance scoring, and no semantic (embedding-based) signal. All three are absent at the retrieval layer, not just the storage layer. A note from three years ago with no links scores identically to a note from yesterday with ten inbound links, if they share the same query terms.
- **Score is opaque SQLite rank, not blendable.** The raw BM25 float cannot be combined with graph proximity or recency without normalization. A blendable score (0–1, with documented semantics) is prerequisite to any multi-signal retrieval improvement.

### Architecture

- **No `Retriever` abstraction for future embedding path.** `SearchFTS` is called directly, not through an interface. Adding an embedding-based retriever in Phase 2 requires forking the call site, not swapping an implementation. The interface should be defined now while the call graph is small.
- **`SearchFTS` is hardcoded, not interface-based.** Same root cause as above. Every caller of `SearchFTS` will need to be updated when a second retrieval backend is added.
- **No schema migration path for embedding column.** There is no migration mechanism in the codebase. Adding a `embedding BLOB` column to the `notes` table in Phase 2 requires a manual schema change with no upgrade path for existing databases.

### Foundation Assessment

- **Foundation is solid.** The schema is correct, the Go architecture is clean, and test coverage is high. The retrieval limitations are design gaps, not implementation failures. They are addressable in Phase 2 with targeted additions.

### Recommendations

1. **Define a `Retriever` interface now** — before Phase 2 adds a second backend. Cost is one interface definition; benefit is clean architecture for all future retrieval work.
2. **Add a schema migration mechanism** — even a simple integer version table with `up` scripts is sufficient. This unblocks the embedding column addition.
3. **Normalize BM25 scores to 0–1** (confirmed with Alex Novak — same fix, different motivation).
4. **Track retrieval frequency** — add `last_accessed_at` and `access_count` to the `notes` table. This is prerequisite data for recency-weighted retrieval in Phase 2.
5. Phase 2 priority: scoring abstraction → schema migration → incremental re-index → embedding extension point.
