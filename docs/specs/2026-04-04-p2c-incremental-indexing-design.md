# P2c: Incremental Indexing — Design Spec

> Phase 2 sub-project C. Fast incremental indexing with mtime+hash change detection and single-file re-index for post-mutation.
>
> SRS references: [12-storage-model.md](../srs/12-storage-model.md), [06-mutation-model.md](../srs/06-mutation-model.md), [08-agent-contract.md](../srs/08-agent-contract.md)

## Goal

Make `vaultmind index` fast by default — skip unchanged files using mtime fast path + hash verification. Add single-file re-indexing for post-mutation use. Detect and remove deleted files.

## Scope

**In scope:**
- `Incremental()` method on `Indexer` — mtime fast path, hash verification, skip unchanged files
- `IndexFile(relPath)` method on `Indexer` — single-file re-index after mutation
- Deleted file detection via set-difference (paths on disk vs paths in DB)
- `index` command defaults to incremental, `--full` flag forces full rebuild
- Wire `IndexFile` into `cmd/frontmatter_helpers.go` after successful mutations
- Extended `IndexResult` with incremental stats (skipped, added, updated, deleted)
- `NoteHashes()` query on DB for bulk mtime+hash lookup

**Out of scope:**
- `--watch` mode (fsnotify)
- Dataview regions (P2d)
- Plan files (P2e)

## Existing Infrastructure

The foundation is already in place:
- `NoteRecord` has `Hash` (SHA-256) and `MTime` (Unix timestamp) fields
- `notes` table has `hash TEXT` and `mtime INTEGER` columns, already populated
- `StoreNote`/`StoreNoteInTx` uses delete-before-reinsert pattern
- `upsertNote` uses `ON CONFLICT(id) DO UPDATE` for hash/mtime
- P2b `Mutator.Run()` sets `ReindexRequired: true` after writes

## Change Detection Strategy

### Mtime Fast Path + Hash Verification (two-pass per file)

For each `.md` file on disk during incremental index:

1. `os.Stat(path)` → get `modTime`
2. Compare to stored `mtime` from DB
3. If mtime unchanged → **skip** (no read, no parse)
4. If mtime changed → `os.ReadFile(path)` → compute SHA-256
5. Compare to stored `hash` from DB
6. If hash unchanged → update mtime in DB, **skip parse**
7. If hash changed → parse + `StoreNote` (full re-index of that file)

This avoids reading file contents for the vast majority of files (those with unchanged mtime), while catching the rare case where mtime changes without content changes (git checkout, touch).

### Deleted File Detection

After processing all files on disk:

1. Collect set of all `relPath` values seen during scan
2. Query `SELECT path FROM notes` for all indexed paths
3. For each DB path not in the disk set → delete that note's records from all tables

Implemented in Go (set difference), no schema changes needed.

## New Methods

### `Indexer.Incremental() (*IndexResult, error)`

```
1. Scan vault (vault.Scan)
2. Query DB for all (path → hash, mtime) via NoteHashes()
3. For each scanned file:
   a. Stat for mtime
   b. If path in DB and mtime matches → skip (Skipped++)
   c. If path in DB and mtime differs → read + hash
      - If hash matches → update mtime only (Skipped++)
      - If hash differs → parse + StoreNote (Updated++)
   d. If path not in DB → parse + StoreNote (Added++)
4. For each DB path not on disk → delete (Deleted++)
5. Run link resolution (same as Rebuild)
6. Return IndexResult with counts
```

### `Indexer.IndexFile(relPath string) error`

```
1. Read file at vaultRoot/relPath
2. Parse (parser.Parse)
3. Build NoteRecord (buildNoteRecord)
4. StoreNote (delete-before-reinsert for that one note)
```

Link resolution is not re-run for single-file — the caller (or a subsequent `index` command) handles that. This keeps `IndexFile` fast for the post-mutation use case.

### `DB.NoteHashes() (map[string]NoteHashInfo, error)`

```go
type NoteHashInfo struct {
    Hash  string
    MTime int64
}
```

Single query: `SELECT path, hash, mtime FROM notes`. Returns map keyed by path.

### `DB.UpdateMTime(path string, mtime int64) error`

Lightweight update for the mtime-changed-but-hash-same case: `UPDATE notes SET mtime = ? WHERE path = ?`. No delete-before-reinsert needed since only the mtime column changes.

## Command Changes

### `vaultmind index`

- Default behavior: calls `Incremental()`
- `--full` flag: calls `Rebuild()` (existing full rebuild)
- JSON response includes incremental stats: `skipped`, `added`, `updated`, `deleted`, `full_rebuild`
- Human-readable output: `Indexed 423 notes (418 skipped, 3 updated, 2 added, 0 deleted) in 0.8s`

### `cmd/frontmatter_helpers.go`

After `m.Run(req)` succeeds and `result.ReindexRequired` is true:

```go
if result.ReindexRequired && !req.DryRun {
    idxr := index.NewIndexer(vaultPath, dbPath, vdb.Config)
    _ = idxr.IndexFile(result.Path)
    result.ReindexRequired = false
}
```

This satisfies the SRS requirement: "After any write, VaultMind performs an implicit incremental re-index of the affected file(s) before returning."

## Extended IndexResult

```go
type IndexResult struct {
    IndexedCount     int           // total files processed (for full rebuild)
    DomainNotes      int
    UnstructuredNotes int
    ErrorCount       int
    DuplicateIDs     int
    Duration         time.Duration
    // New incremental fields:
    Skipped          int           // files unchanged
    Added            int           // new files indexed
    Updated          int           // changed files re-indexed
    Deleted          int           // removed files cleaned up
    FullRebuild      bool          // true if --full was used
}
```

## Testing Strategy

### Unit tests

- **NoteHashes query** — index a vault, call NoteHashes, verify paths/hashes/mtimes match
- **Incremental skip** — index, re-index without changes → all files skipped
- **Incremental update** — index, modify one file, re-index → Skipped=N-1, Updated=1
- **Incremental add** — index, add new file, re-index → Added=1
- **Incremental delete** — index, delete file, re-index → Deleted=1
- **Mtime-only change** — index, touch file without content change, re-index → mtime updated, Skipped (not Updated)
- **IndexFile** — mutate a file, call IndexFile, verify DB reflects change
- **Full flag** — verify `--full` calls Rebuild

### Integration tests

- **Post-mutation re-index** — run `frontmatter set`, verify the mutated note is immediately queryable via `note get` without a separate `index` call

### Coverage target

85%+ for new code in `internal/index/`.

## Design Decisions

### DD-1: Incremental by default

**Choice:** `vaultmind index` does incremental. `--full` forces full rebuild.

**Rationale:** The whole point of P2c is fast indexing. Agents should get the fast path by default. `--full` exists for recovery (corrupted DB, migration).

### DD-2: Mtime fast path + hash verification

**Choice:** Skip `os.ReadFile` when mtime unchanged. Only hash when mtime differs.

**Rationale:** On a 500-note vault, skipping file reads for 490 unchanged files is a significant speedup. The mtime-changed-but-hash-same case (git checkout) just wastes one hash computation — not a correctness issue.

### DD-3: Set-difference for deletion detection

**Choice:** Compare disk paths vs DB paths in Go. No schema changes.

**Rationale:** The `notes` table already has `path`. A simple set operation detects orphans. No new columns or migration needed.

### DD-4: Command layer triggers post-mutation re-index

**Choice:** `frontmatter_helpers.go` calls `IndexFile` after mutation, not the Mutator itself.

**Rationale:** Keeps mutation package decoupled from indexing. The command layer is already the integration point. Mutator doesn't need an Indexer dependency.

## File Inventory

| File | Change |
|------|--------|
| `internal/index/indexer.go` | Add `Incremental()`, `IndexFile()` methods |
| `internal/index/db.go` | Add `NoteHashes()` query, `NoteHashInfo` type |
| `internal/index/indexer_test.go` | Tests for Incremental + IndexFile |
| `internal/index/db_test.go` | Test for NoteHashes |
| `cmd/index.go` | Add `--full` flag, default to incremental |
| `cmd/frontmatter_helpers.go` | Wire IndexFile after mutations |
| `internal/config/commands/index_config.go` | Add `full` flag config option |
| `.ckeletin/pkg/config/keys_generated.go` | Regenerated with new constant |
