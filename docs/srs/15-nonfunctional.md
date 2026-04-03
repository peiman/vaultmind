# Non-functional Requirements

> See also: [storage model](12-storage-model.md), [safety](14-safety-model.md)

## Performance Targets

| Operation | Target |
|-----------|--------|
| Initial indexing, 10,000 notes | < 30 seconds |
| Incremental indexing, 10,000 notes with 10 changes | < 2 seconds |
| Single-note queries (resolve, get, links) | < 100ms |
| Memory recall at depth 2 | < 500ms |
| Context-pack assembly | < 1 second |

All targets assume commodity hardware (e.g., M1 MacBook Air, mid-range Linux server).

## Reliability

- Index runs are idempotent given unchanged inputs
- Incremental re-indexing updates only changed files
- Writes are atomic at file level
- Plan-file execution uses best-effort rollback (see [safety model](14-safety-model.md) for failure modes)

## Portability

- macOS and Linux (amd64, arm64)
- Direct local filesystem access — no network dependencies for core operations
- No dependency on Obsidian runtime

## YAML Parser Requirements

- Must use a YAML 1.2-compatible parser in strict mode
- String scalars must not be silently coerced to booleans (`yes`/`no`/`true`/`false`/`on`/`off` must remain strings)
- Block scalars (`|`, `>`) must be round-tripped unchanged during frontmatter writes
- Go implementation: `go-yaml v3` with strict unmarshaling
- Multiline string fields and special characters must be preserved byte-for-byte

## Determinism

- Repeated runs on unchanged vault produce identical index state
- Generated Dataview blocks are reproducible
- YAML serialization preserves key order and avoids non-deterministic formatting

## Compatibility

### Must be compatible with

- Obsidian-compatible Markdown vaults
- YAML frontmatter as used by Obsidian Properties
- Obsidian wikilink syntax (`[[...]]`, `![[...]]`)
- Common heading and block reference conventions
- Dataview fenced blocks as text regions
- Git repositories on local filesystems

### Must not depend on

- Obsidian desktop runtime or plugin APIs
- Dataview execution engine
- Cloud-hosted storage
- Any external database or service
