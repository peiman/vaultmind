# Mutation Model

> See also: [git model](07-git-model.md), [safety](14-safety-model.md), [validation rules](13-validation-rules.md), [plan files](10-plan-files.md)

## Allowed Mutation Surfaces

| Surface | Operations |
|---------|-----------|
| Frontmatter | `set`, `unset`, `merge`, `normalize`, `validate` |
| Generated regions | Replace content between explicit VAULTMIND markers |
| New notes | Create from registered templates via `note create` |

## Forbidden Mutation Surfaces

- Human-authored body prose outside managed regions
- Freeform summarization injected into existing prose
- Rewrites when markers are malformed or duplicated
- Broad formatting rewrites of whole notes

## Mutation Workflow

All write operations follow plan-then-apply:

1. **Read** current file state (read content, compute hash)
2. **Compute** bounded intended change
3. **Preview** generate unified diff
4. **Validate** safety conditions (schema, markers, Git state)
5. **Verify** file unchanged on disk since read (compare hash)
6. **Write** atomically (write to temp file, rename)
7. **Commit** optionally stage and commit

## Write Guarantees

- Atomic at file level (temp-file + rename)
- Preserve unrelated content byte-for-byte
- Preserve YAML key order during frontmatter writes
- Minimize noisy diffs (no unnecessary re-serialization)
- Fail on ambiguity rather than guess
- Preserve newline conventions (detect and maintain LF vs CRLF)
- Preserve trailing newline state

## Frontmatter Normalize

The `frontmatter normalize` command applies these transformations:

- Sort frontmatter keys into canonical order: `id`, `type`, `status`, `title`, `aliases`, `tags`, `created`, `updated`, then remaining keys alphabetically
- Convert scalar `aliases`/`tags` to single-element lists
- Normalize date fields to `YYYY-MM-DD` format only when time component is `T00:00:00` (opt-in: `--strip-time` forces all datetimes to date-only)
- Collapse multiple blank lines between frontmatter and body to exactly one
- Convert non-snake_case keys to snake_case (with `--dry-run` showing renames)

All normalize operations are individually skippable via flags.

## Mutation Refusal Rules

| Condition | Error code | Behavior |
|-----------|-----------|----------|
| Target file changed on disk since read | `conflict` | Refuse |
| Frontmatter key not recognized by type schema | `unknown_key` | Refuse (unless `--allow-extra`) |
| `id` or `type` field targeted for modification | `immutable_field` | Refuse unconditionally |
| Generated region markers malformed or duplicated | `marker_error` | Refuse |
| Generated region hand-edited (checksum mismatch) | `checksum_mismatch` | Refuse (unless `--force`) |
| Git policy violation | per [git model](07-git-model.md) | Refuse or warn per matrix |
| Note is unstructured and operation requires domain note | `not_domain_note` | Refuse |

## Dataview Generated Regions

Dataview is a managed presentation layer, not a canonical runtime.

### Generated Region Convention

```markdown
## Related notes

<!-- VAULTMIND:GENERATED:related:START -->
```dataview
TABLE WITHOUT ID link(file.name, title) AS Note, status, updated
FROM "projects"
WHERE contains(related_ids, this.id)
SORT updated DESC
```
<!-- VAULTMIND:GENERATED:related:END -->
```

**Marker format:** `<!-- VAULTMIND:GENERATED:{section_key}:START -->` / `<!-- VAULTMIND:GENERATED:{section_key}:END -->`

`section_key` must be a lowercase alphanumeric slug. Used for idempotent replacement.

### Safety Rules for Generated Regions

- Refuse if START exists without matching END
- Refuse if markers are duplicated within same file
- Refuse if content between markers was hand-edited since last generation (checksum mismatch) — warn and require `--force`

### Supported Dataview Operations (v1)

- Detection of `dataview` and `dataviewjs` fenced blocks
- Insertion/replacement of generated regions between markers
- Templating of approved snippets per note type
- Linting of managed regions

### Not Supported (v1)

- Full Dataview query execution
- Arbitrary `dataviewjs` static analysis
- Treating Dataview output as canonical data
