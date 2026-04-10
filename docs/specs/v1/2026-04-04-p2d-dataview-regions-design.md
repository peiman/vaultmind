# P2d: Dataview Generated Regions — Design Spec

> Phase 2 sub-project D. Managed generated regions with marker-based replacement, checksum hand-edit detection, and linting.
>
> SRS references: [06-mutation-model.md](../srs/06-mutation-model.md) (Generated Regions section), [13-validation-rules.md](../srs/13-validation-rules.md), [11-cli-reference.md](../srs/11-cli-reference.md), [14-safety-model.md](../srs/14-safety-model.md)

## Goal

Implement `dataview render` and `dataview lint` commands for managing generated regions in vault notes. Generated regions are delimited by `<!-- VAULTMIND:GENERATED:{key}:START/END -->` markers. Content between markers is replaced with static section templates. Checksum-based hand-edit detection prevents accidental overwrites.

## Scope

**In scope:**
- `internal/marker/` package: parse, detect, validate, and replace generated region markers
- `dataview render` command: replace content between markers with section template
- `dataview lint` command: validate marker integrity across files
- Checksum-based hand-edit detection (self-contained checksum comment in marker block)
- Section template files at `.vaultmind/sections/{type}/{key}.md`
- `--force` flag to override checksum mismatch
- Integration with git policy, atomic writes, post-write re-index

**Out of scope:**
- Dataview query execution
- Dynamic Go template rendering (v1 uses static snippets)
- `dataviewjs` static analysis
- Treating Dataview output as canonical data

## Marker Format

```markdown
<!-- VAULTMIND:GENERATED:{section_key}:START -->
<!-- checksum:{sha256_hex} -->
{content from section template}
<!-- VAULTMIND:GENERATED:{section_key}:END -->
```

- `section_key` is a lowercase alphanumeric slug (e.g., `related`, `backlinks`)
- Checksum line is inserted by VaultMind after the START marker
- Checksum covers the content bytes between the checksum line and the END marker
- On first insertion (no prior checksum), the region is always writable

## Package: `internal/marker/`

### Types

```go
// Marker represents a detected generated region in a file.
type Marker struct {
    SectionKey   string
    StartOffset  int    // byte offset of START comment line
    EndOffset    int    // byte offset after END comment line (including trailing newline)
    ContentStart int    // byte offset of content after checksum line
    ContentEnd   int    // byte offset of END comment line
    Checksum     string // stored checksum from comment (empty if none)
    Content      string // current content between checksum line and END marker
}

// Issue represents a marker validation problem.
type Issue struct {
    SectionKey string `json:"section_key,omitempty"`
    Rule       string `json:"rule"`
    Message    string `json:"message"`
    Line       int    `json:"line"`
}
```

### Functions

```go
// FindMarkers scans raw file bytes for all VAULTMIND:GENERATED marker pairs.
// Returns markers sorted by offset. Returns error if unpaired START found.
func FindMarkers(raw []byte) ([]Marker, error)

// ValidateMarkers checks for malformed/duplicated markers. Returns issues.
func ValidateMarkers(raw []byte) []Issue

// ReplaceRegion replaces the content between markers for a given section_key.
// Inserts a checksum comment after START. Returns the new file bytes.
// Returns checksum_mismatch error if hand-edited and force is false.
func ReplaceRegion(raw []byte, sectionKey string, newContent []byte, force bool) ([]byte, error)

// ContentChecksum computes SHA-256 hex of the content bytes.
func ContentChecksum(content []byte) string
```

### Key Behaviors

- `FindMarkers` uses regex to find `<!-- VAULTMIND:GENERATED:{key}:START -->` and matching END markers. Parses optional `<!-- checksum:{hash} -->` line after START.
- `ValidateMarkers` checks: unpaired START/END, duplicate section_keys in same file, END without START.
- `ReplaceRegion`:
  1. Find marker pair for `sectionKey` (error if not found)
  2. If marker has a stored checksum, compute `ContentChecksum(currentContent)` and compare
  3. If mismatch and `!force`, return `MutationError{Code: "checksum_mismatch"}`
  4. Build replacement: START line + checksum comment + newContent + END line
  5. Splice into raw bytes: `raw[:marker.StartOffset] + replacement + raw[marker.EndOffset:]`
  6. Return new bytes

## Section Templates

Templates are plain markdown files at `.vaultmind/sections/{type}/{key}.md`.

```
.vaultmind/
  sections/
    project/
      related.md
      backlinks.md
    concept/
      related.md
```

Each file contains the raw content to insert between markers (typically a Dataview query block). No Go template processing — static snippets only (DD-1).

### Template Loading

```go
// LoadSectionTemplate reads the section template for a given note type and section key.
// Looks for .vaultmind/sections/{type}/{key}.md in the vault root.
func LoadSectionTemplate(vaultRoot, noteType, sectionKey string) ([]byte, error)
```

Returns `MutationError{Code: "template_not_found"}` if the file doesn't exist.

## Render Pipeline

The `RenderRegion` function orchestrates the full render workflow:

```go
func RenderRegion(cfg RenderConfig) (*RenderResult, error)

type RenderConfig struct {
    VaultPath  string
    Target     string // path or id
    SectionKey string // which section to render (empty = all sections in file)
    DryRun     bool
    Diff       bool
    Commit     bool
    Force      bool  // override checksum mismatch
}

type RenderResult struct {
    Path            string `json:"path"`
    ID              string `json:"id"`
    SectionKey      string `json:"section_key"`
    Operation       string `json:"operation"` // "dataview_render"
    DryRun          bool   `json:"dry_run"`
    Diff            string `json:"diff,omitempty"`
    WriteHash       string `json:"write_hash,omitempty"`
    Git             GitInfo `json:"git"`
    ReindexRequired bool   `json:"reindex_required"`
    Warnings        []PolicyWarning `json:"warnings"`
}
```

Steps:
1. Resolve target to file path (same path-based resolution as mutations)
2. Read file, compute hash
3. Determine note type from frontmatter
4. Load section template from `.vaultmind/sections/{type}/{key}.md`
5. Find markers in the note body
6. Check hand-edit checksum (refuse if mismatch, unless `--force`)
7. Replace region content with template + new checksum
8. Generate diff if requested
9. If dry-run, return result with diff
10. Check git policy, verify file unchanged, atomic write
11. Post-write re-index via `IndexFile`

This reuses types from `internal/mutation/` (`GitInfo`, `PolicyWarning`, `MutationError`) and git policy checking from `internal/git/`.

## CLI Commands

### `dataview render`

```
vaultmind dataview render <path-or-id> [--section-key KEY] [--dry-run] [--diff] [--commit] [--force] [--json]
```

- Without `--section-key`: renders all sections found in the file
- With `--section-key`: renders only the specified section
- `--force`: override checksum mismatch (hand-edited content will be overwritten)
- Other flags match mutation commands

### `dataview lint`

```
vaultmind dataview lint [<path-or-glob>] [--json]
```

- Without arguments: lints all indexed notes
- With path/glob: lints specified files
- Returns issue list with `malformed_markers` and `duplicate_markers` rules
- JSON response matches `frontmatter validate` shape (files_checked, valid, issues)

### Command Files

- `cmd/dataview.go` — parent command
- `cmd/dataview_render.go` — render subcommand (thin, delegates to `RenderRegion`)
- `cmd/dataview_lint.go` — lint subcommand

## Validation Rules

Two rules from the SRS, surfaced by `dataview lint` and also checked during `dataview render`:

| Rule | Condition | Severity |
|------|-----------|----------|
| `malformed_markers` | START without matching END, or END without START | error |
| `duplicate_markers` | Same `section_key` appears more than once in a file | error |

## Testing Strategy

### Unit tests (no disk)

- **FindMarkers**: file with 0 markers, 1 marker pair, 2 marker pairs, unpaired START (error), END without START (error), marker with checksum, marker without checksum
- **ValidateMarkers**: clean file, malformed markers, duplicated section keys
- **ReplaceRegion**: replace existing content, verify checksum inserted, verify body outside markers untouched
- **Hand-edit detection**: modify content between markers, verify `checksum_mismatch` error, verify `--force` overrides
- **ContentChecksum**: deterministic hash for same content

### Integration tests (temp vault)

- **RenderRegion**: create vault with config, section template, and note with markers. Render and verify content replaced.
- **Missing template**: render with no template file, verify `template_not_found` error
- **Missing markers**: render on note without markers, verify error
- **Dry-run**: verify file untouched, diff returned
- **Lint**: vault with mixed valid/invalid marker files, verify correct issue count

### Coverage target

85%+ for `internal/marker/` package.

## Design Decisions

### DD-1: Static template snippets

**Choice:** v1 uses pre-defined templates from files, not dynamic Go template rendering.

**Rationale:** SRS says "templating of approved snippets." Static files are predictable, auditable, and agent-friendly. Dynamic rendering adds complexity and security surface with no clear v1 need.

### DD-2: Separate section template files

**Choice:** Templates at `.vaultmind/sections/{type}/{key}.md`, not inline in config.yaml.

**Rationale:** Section templates contain markdown with fenced code blocks. Embedding in YAML is fragile. Separate files let users edit templates in Obsidian directly.

### DD-3: Self-contained checksum in file

**Choice:** Checksum stored as `<!-- checksum:{hash} -->` comment inside the marker block.

**Rationale:** No dependency on index state. Checksum travels with the file. Moving vaults, rebuilding indexes, or copying files preserves the checksum. The `generated_sections` DB table can store metadata for querying, but the file is authoritative.

## File Inventory

| File | Purpose |
|------|---------|
| `internal/marker/marker.go` | `Marker` type, `FindMarkers`, `ContentChecksum` |
| `internal/marker/validate.go` | `ValidateMarkers`, `Issue` type |
| `internal/marker/replace.go` | `ReplaceRegion` (splice + checksum) |
| `internal/marker/render.go` | `RenderRegion` orchestrator, `RenderConfig`, `RenderResult`, `LoadSectionTemplate` |
| `internal/marker/marker_test.go` | FindMarkers + ContentChecksum tests |
| `internal/marker/validate_test.go` | ValidateMarkers tests |
| `internal/marker/replace_test.go` | ReplaceRegion + hand-edit detection tests |
| `internal/marker/render_test.go` | Integration tests for full render pipeline |
| `cmd/dataview.go` | Parent `dataview` command |
| `cmd/dataview_render.go` | `dataview render` subcommand |
| `cmd/dataview_lint.go` | `dataview lint` subcommand |
| `internal/config/commands/dataview_config.go` | Config registration for render + lint |
