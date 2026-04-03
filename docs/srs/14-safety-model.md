# Safety Model

> See also: [mutation model](06-mutation-model.md), [git model](07-git-model.md)

## General Safety

- Never rewrite human-authored prose outside managed regions
- Never treat inferred associations as canonical facts
- Never silently resolve ambiguous identity collisions
- Never write when managed markers are malformed or duplicated

## Repository Safety

- Enforce [Git policy matrix](07-git-model.md) before all mutations
- Stage only files modified by the current operation
- Keep mutation scopes narrow — one logical change per commit

## File Safety

- Atomic writes via temp-file + rename
- Verify file hash before overwrite (detect changed-on-disk)
- Preserve newline conventions (detect and maintain LF vs CRLF)
- Preserve trailing newline state

## Concurrency Model

VaultMind v1 assumes **single-writer access**. No file locking.

Concurrent access (e.g., human editing in Obsidian while agent runs VaultMind) is handled via hash-based conflict detection: if the file changed between read and write, the mutation is refused with a `conflict` error.

No retry logic — the caller decides whether to re-read and retry.

### `--watch` Concurrency

`index --watch` and agent mutations must not run concurrently. `--watch` is a human-facing mode for live vault monitoring. If a VaultMind mutation process is active, `--watch` should not re-index the target file until the mutation completes. In practice: agents should not use `--watch`; they should call `index` explicitly when needed.

## Failure Modes

| Failure | Behavior |
|---------|----------|
| File changed on disk during mutation | Refuse with `conflict` |
| Disk full during atomic write | Temp file write fails, original untouched |
| Process killed during atomic write | Temp file orphaned, original untouched |
| Process killed after rename, before commit | File updated, Git state dirty — safe to re-run |
| SQLite index corrupted | `vaultmind index` rebuilds from vault — no data loss |
| Plan execution fails mid-way | Best-effort rollback from pre-operation copies |
| Process killed during plan execution | Partial writes may persist; `vaultmind index` reflects actual state |
| `--force` override on checksum mismatch | Write proceeds; `warning` emitted in JSON envelope; ineligible for `--commit` without explicit `--commit --force` |
| Git pre-commit hook failure | File written to disk, commit rejected; working tree left dirty; next mutation may refuse if target has staged changes |
