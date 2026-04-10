# P3a: Inferred Edges — Design Spec

> Phase 3 sub-project A. Alias mention and tag overlap edge detection during indexing.
>
> SRS references: [05-memory-model.md](../srs/05-memory-model.md), [12-storage-model.md](../srs/12-storage-model.md), [18-config-spec.md](../srs/18-config-spec.md)

## Goal

Compute `alias_mention` and `tag_overlap` inferred edges during indexing and store them in the existing `links` table. This enables `memory related` (P3c) to return inferred associations alongside explicit edges.

## Scope

**In scope:**
- `ComputeAliasMentions(db, minAliasLen)` — scan note bodies for unlinked mentions of known aliases/titles
- `ComputeTagOverlap(db, threshold)` — IDF-weighted tag overlap scoring between note pairs
- `StripForAliasMatch(body)` — remove code fences, inline code, wikilinks, HTML comments
- Wire both into `Rebuild()` and `Incremental()` as post-index passes
- Store edges in existing `links` table with appropriate edge_type and confidence

**Out of scope:**
- Graph traversal (P3b)
- Memory commands (P3c)
- `dataview_reference` edge type (not required for v1 memory)

## Alias Mention Detection

```go
// ComputeAliasMentions scans all note bodies for unlinked mentions of known
// aliases and titles. Creates alias_mention edges in the links table.
// Returns the number of edges created.
func ComputeAliasMentions(db *DB, minAliasLen int) (int, error)
```

### Algorithm

1. Clear old `alias_mention` edges: `DELETE FROM links WHERE edge_type = 'alias_mention'`
2. Query all aliases and titles with their note IDs:
   ```sql
   SELECT note_id, alias FROM aliases
   UNION
   SELECT id, title FROM notes WHERE is_domain = TRUE AND title != ''
   ```
3. Filter aliases shorter than `minAliasLen` (default 3 from config)
4. Build a single compiled regex: `(?i)\b(escaped_alias1|escaped_alias2|...)\b`
5. For each note body (from `notes.body_text`):
   - Strip code fences, inline code, wikilinks, HTML comments via `StripForAliasMatch`
   - Run regex against stripped body
   - For each match, resolve which alias it belongs to → get target note ID
   - Skip self-references (source note ID == target note ID)
   - Deduplicate: one edge per (source, target) pair
6. Batch insert edges:
   ```sql
   INSERT OR IGNORE INTO links (src_note_id, dst_note_id, edge_type, confidence, origin)
   VALUES (?, ?, 'alias_mention', 'medium', 'body:alias_scan')
   ```

### Matching Rules (from SRS)

- Word-boundary matching only (no partial-word matches) — `\b` in regex
- Case-insensitive comparison — `(?i)` flag
- Minimum alias length: configurable, default 3
- Skip matches inside code fences, YAML frontmatter, and existing wikilinks
- One edge per source-target pair (not per occurrence)

## Tag Overlap Detection

```go
// ComputeTagOverlap computes IDF-weighted tag overlap scores between note pairs
// and creates tag_overlap edges where the score exceeds the threshold.
// Returns the number of edges created.
func ComputeTagOverlap(db *DB, threshold float64) (int, error)
```

### Algorithm

1. Clear old `tag_overlap` edges: `DELETE FROM links WHERE edge_type = 'tag_overlap'`
2. Count total domain notes: `SELECT COUNT(*) FROM notes WHERE is_domain = TRUE`
3. Count notes per tag: `SELECT tag, COUNT(*) FROM tags GROUP BY tag`
4. Compute specificity per tag: `log(totalDomainNotes / notesWithTag)`
5. Find all note pairs sharing at least one tag:
   ```sql
   SELECT a.note_id, b.note_id, a.tag
   FROM tags a JOIN tags b ON a.tag = b.tag AND a.note_id < b.note_id
   ```
6. For each pair, sum specificities of shared tags → `overlap_score`
7. If `overlap_score >= threshold` (default 1.0 from config), insert edge:
   ```sql
   INSERT OR IGNORE INTO links (src_note_id, dst_note_id, edge_type, confidence, origin, weight)
   VALUES (?, ?, 'tag_overlap', 'low', 'tag_overlap_scan', ?)
   ```
   (The `weight` column stores the overlap score)

### Scoring Formula (from SRS)

```
specificity(tag) = log(total_domain_notes / notes_with_tag)
overlap_score(A, B) = sum(specificity(tag) for tag in shared_tags(A, B))
```

## Body Text Stripping

```go
// StripForAliasMatch removes regions from body text that should not be scanned
// for alias mentions: code fences, inline code, wikilinks, HTML comments.
func StripForAliasMatch(body string) string
```

Removes:
- Fenced code blocks (` ```...``` ` including content)
- Inline code (`` `...` ``)
- Wikilink targets (`[[target]]` → remove entirely; `[[target|display]]` → keep display text)
- HTML comments (`<!-- ... -->`)

The remaining text is what gets scanned for alias matches.

## Indexer Integration

Both functions are called at the end of `Rebuild()` and `Incremental()`, after link resolution:

```go
// After ResolveLinks:
aliasCount, aliasErr := ComputeAliasMentions(db, idx.cfg.Memory.AliasMinLength)
if aliasErr != nil {
    log.Debug().Err(aliasErr).Msg("alias mention detection failed")
} else {
    log.Debug().Int("edges", aliasCount).Msg("alias mention detection complete")
}

tagCount, tagErr := ComputeTagOverlap(db, idx.cfg.Memory.TagOverlapThreshold)
if tagErr != nil {
    log.Debug().Err(tagErr).Msg("tag overlap detection failed")
} else {
    log.Debug().Int("edges", tagCount).Msg("tag overlap detection complete")
}
```

Config values from `vault.Config.Memory`:
- `AliasMinLength` — default 3 (minimum alias length for mention detection)
- `TagOverlapThreshold` — default 1.0 (minimum overlap score for edge creation)

## Testing Strategy

### Unit tests

- **StripForAliasMatch**: code fence removal, inline code removal, wikilink removal (both plain and aliased), HTML comment removal, preserves normal text
- **AliasMention basic**: note A has alias "Retry Engine", note B body contains "the Retry Engine handles..." → creates edge from B to A
- **AliasMention skip code**: alias inside fenced code block → no edge
- **AliasMention skip wikilink**: `[[Retry Engine]]` in body → no edge (explicit link already exists)
- **AliasMention self-reference**: note mentions its own alias → no edge
- **AliasMention min length**: alias "AB" (2 chars) with minAliasLen=3 → skipped
- **AliasMention case-insensitive**: "retry engine" matches "Retry Engine" alias
- **AliasMention dedup**: alias appears 3 times in body → only one edge

### Integration tests

- **TagOverlap basic**: two notes sharing tag "billing" with sufficient specificity → edge created
- **TagOverlap threshold**: two notes sharing only a very common tag → score below threshold, no edge
- **TagOverlap scoring**: verify score matches IDF formula
- **Full Rebuild**: index test vault, verify both `alias_mention` and `tag_overlap` edges in links table
- **Incremental**: modify a note, re-index, verify inferred edges recomputed

### Coverage target

85%+ for new code in `internal/index/inferred.go`.

## Design Decisions

### DD-1: Post-index pass

**Choice:** Compute inferred edges after parsing, store in links table.

**Rationale:** SRS says "Indexer" produces these edges. Storing in the same `links` table means all query commands (`links in/out`, `memory recall`, `memory related`) work uniformly — no special-case logic at query time.

### DD-2: Simple regex matching

**Choice:** Build one compiled regex from all aliases, run against each note body.

**Rationale:** O(notes × regex_match) is fast enough for <1,000 notes. Go's regexp engine handles 2,000-alternative patterns in milliseconds. YAGNI — optimize to Aho-Corasick only if profiling shows need.

### DD-3: In index package

**Choice:** All inferred edge logic in `internal/index/`.

**Rationale:** Follows SRS component assignment. Same table, same package. The memory package (P3c) will be query-only.

## File Inventory

| File | Change |
|------|--------|
| `internal/index/inferred.go` | New: `ComputeAliasMentions`, `ComputeTagOverlap`, `StripForAliasMatch` |
| `internal/index/inferred_test.go` | New: all tests for inferred edge logic |
| `internal/index/indexer.go` | Modify: wire calls into `Rebuild()` and `Incremental()` |
