# CLI Reference

> See also: [response shapes](09-response-shapes.md), [agent contract](08-agent-contract.md)

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--vault <path>` | Current directory or config | Path to vault root |
| `--config <path>` | `.vaultmind/config.yaml` | Path to config file |
| `--json` | auto | Machine-readable JSON output. **Defaults to `true` when stdout is not a TTY** (`isatty` detection). Use `--no-json` to force human output in pipes. |
| `--verbose` | false | Detailed logging to stderr |
| `--quiet` | false | Suppress warnings |

## Commands

### Index and Query

```
vaultmind index [--watch] [--json]
```
Rebuild or update the SQLite index. `--watch` uses fsnotify to re-index on file changes (logs to stderr, runs until interrupted).

```
vaultmind search <query> [--type TYPE] [--tag TAG] [--limit N] [--offset N] [--json]
```
Full-text search. Default `--limit 20`, `--offset 0`.

```
vaultmind resolve <id-or-title-or-alias> [--json]
```
Run entity resolution, return matches with tier.

### Notes

```
vaultmind note get <id-or-path> [--frontmatter-only] [--json]
vaultmind note mget [--ids id1,id2,...] [--stdin] [--frontmatter-only] [--json]
```
`get` returns full note content and metadata. `--frontmatter-only` omits body, headings, and blocks. `mget` accepts multiple IDs (comma-separated via `--ids`, or newline-delimited on stdin) and returns an array of note objects. Defaults to `--frontmatter-only`.

```
vaultmind note create <path> --type <type> [--field key=value ...] [--body <text>] [--json]
```
Create note from template. `--field` sets frontmatter fields. `--body` sets body text (overrides template body).

### Links

```
vaultmind links out <id-or-path> [--edge-type TYPE] [--json]
vaultmind links in <id-or-path> [--edge-type TYPE] [--json]
vaultmind links neighbors <id-or-path> [--depth N] [--min-confidence low|medium|high] [--max-nodes N] [--json]
```
`--edge-type` filters to a single edge type. `neighbors` returns raw edge list (see [memory model](05-memory-model.md) for distinction from `memory recall`).

### Memory

```
vaultmind memory recall <id-or-path> [--depth N] [--min-confidence low|medium|high] [--max-nodes N] [--json]
vaultmind memory related <id-or-path> [--mode explicit|inferred|mixed] [--json]
vaultmind memory context-pack <id-or-path> [--budget N] [--json]
```
`recall` returns enriched graph neighborhood (frontmatter only, no body text). `--max-nodes` caps result set (default: 200). `related` returns flat list filtered by mode. `context-pack` assembles token-budgeted payload.

### Frontmatter

```
vaultmind frontmatter validate [<path-or-glob>] [--json]
vaultmind frontmatter set <path-or-id> <key> <value> [--dry-run] [--diff] [--commit] [--allow-extra] [--json]
vaultmind frontmatter unset <path-or-id> <key> [--dry-run] [--diff] [--commit] [--json]
vaultmind frontmatter merge <path-or-id> --file <yaml-file> [--dry-run] [--diff] [--commit] [--allow-extra] [--json]
vaultmind frontmatter normalize [<path-or-glob>] [--dry-run] [--diff] [--commit] [--json]
```

### Dataview

```
vaultmind dataview render <path-or-id> [--section-key KEY] [--dry-run] [--diff] [--commit] [--force] [--json]
vaultmind dataview lint [<path-or-glob>] [--json]
```

### Git

```
vaultmind git status [--json]
```
Report repo state relevant to VaultMind policies.

### Plan Execution

```
vaultmind apply <plan-file | -> [--dry-run] [--diff] [--commit] [--json]
```
Accepts a plan file path or `-` to read from stdin. See [plan files](10-plan-files.md).

### Diagnostics

```
vaultmind doctor [--json]
```
Vault health summary.

### Schema Introspection

```
vaultmind schema list-types [--json]
```
Returns the type registry in machine-readable form: type names, required/optional fields, valid statuses, and template paths. Enables agents to discover the schema without reading config files.

### Field Value Syntax

For `--field key=value` on `note create`:
- String values: `--field title="My Note"`
- List values: `--field tags='["billing", "payments"]'` (JSON array syntax)
- Repeated `--field` flags set multiple fields, they do not append to the same key

## Target Resolution

Any command accepting `<id-or-path>` or `<path-or-id>` runs [entity resolution](03-data-model.md). If the input contains `/` or ends in `.md`, path lookup is tried first. If ambiguous, error with candidates.
