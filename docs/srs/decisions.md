# Design Decisions — v2 to v3

> Changes made during the v2 review and v3 restructuring.

## Issues Found in v2 and Resolutions

### Gaps Filled

| # | Gap | Resolution | Affected files |
|---|-----|-----------|---------------|
| 1 | No `note delete` command | Explicitly listed as non-goal with rationale (use Git directly) | [01](01-overview.md) |
| 2 | No `note move`/`note rename` | Explicitly listed as non-goal (path is non-canonical; use filesystem) | [01](01-overview.md) |
| 3 | `search` lacks pagination | Added `--limit` and `--offset` flags, reflected in response shape | [11](11-cli-reference.md), [09](09-response-shapes.md) |
| 4 | FTS5 schema broken | Fixed: standalone FTS5 table, added `body_text` column to `notes`, integer rowid | [12](12-storage-model.md) |
| 5 | `frontmatter normalize` undefined | Defined: key ordering, list conversion, date normalization, snake_case | [06](06-mutation-model.md) |
| 6 | `memory related` underspecified | Added response shape and mode definitions | [09](09-response-shapes.md), [05](05-memory-model.md) |
| 7 | Template system unspecified | New spec created | [19](19-template-spec.md) |
| 8 | Config file location/schema missing | New spec created | [18](18-config-spec.md) |
| 9 | `--watch` mechanism unspecified | Specified: fsnotify, logs to stderr, runs until interrupted | [11](11-cli-reference.md) |
| 10 | Human-readable output format undefined | Clarified as unstable, not for programmatic use | [08](08-agent-contract.md) |

### Ambiguities Resolved

| # | Issue | Resolution | Affected files |
|---|-------|-----------|---------------|
| 11 | `links neighbors` vs `memory recall` overlap | Clarified: `neighbors` = raw edges, `recall` = enriched nodes with frontmatter | [05](05-memory-model.md), [11](11-cli-reference.md) |
| 12 | Unstructured notes primary key in SQLite | Defined synthetic key: `_path:<vault-relative-path>` | [03](03-data-model.md), [12](12-storage-model.md) |
| 13 | `frontmatter_kv` purpose unclear | Clarified: stores domain-tier fields not in dedicated columns | [12](12-storage-model.md) |
| 14 | Alias mention detection rules missing | Specified: word boundaries, case-insensitive, min length 3, skip code fences | [05](05-memory-model.md) |
| 15 | Tag overlap specificity formula missing | Added IDF-based formula with threshold | [05](05-memory-model.md) |
| 16 | FTS rowid mismatch | Fixed with `INTEGER PRIMARY KEY AUTOINCREMENT` on `notes` | [12](12-storage-model.md) |
| 17 | `blocks.end_line` nullable unexplained | Clarified: NULL for single-line blocks | [12](12-storage-model.md) |
| 18 | Plan `note_create` missing `body` | Added optional `body` field | [10](10-plan-files.md) |

### Additions

| Addition | Rationale | File |
|----------|----------|------|
| Exit codes | Agents need predictable process exit behavior | [08](08-agent-contract.md) |
| Mutation error codes | Stable identifiers for programmatic error handling | [06](06-mutation-model.md) |
| Plan validation errors | Specific error codes for plan file issues | [10](10-plan-files.md) |
| Validation rule IDs | Machine-readable identifiers for each rule | [13](13-validation-rules.md) |
| `weight` column on `links` | Needed for tag_overlap scores | [12](12-storage-model.md) |
| `--body` flag on `note create` | Override template body from CLI | [11](11-cli-reference.md) |
| `--allow-extra` flag on `frontmatter set` | Escape hatch for unrecognized fields | [11](11-cli-reference.md) |
| `--force` flag on `dataview render` | Override checksum mismatch protection | [11](11-cli-reference.md) |
| Failure modes table | Explicit behavior for crash/disk/corruption scenarios | [14](14-safety-model.md) |
| Short alias risk | Added to risks table | [16](16-risks.md) |
