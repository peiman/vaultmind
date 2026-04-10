# P3d: Note Create — Design Spec

> Phase 3 sub-project D (final). Template-based note creation with variable substitution.
>
> SRS references: [19-template-spec.md](../srs/19-template-spec.md), [11-cli-reference.md](../srs/11-cli-reference.md), [13-validation-rules.md](../srs/13-validation-rules.md)

## Goal

Implement the `note create` CLI command with template loading, `<%=variable%>` substitution, ID generation, field overrides, and validation. This is the final sub-project — it completes Phase 3 and VaultMind v1.

## Scope

**In scope:**
- `internal/template/` package: template loading, variable substitution, ID generation
- `note create` CLI command with `--type`, `--field`, `--body`, `--commit` flags
- `<%=variable%>` placeholder substitution (8 variables from SRS)
- ID generation: `{type}-{slug}` from filename, uniqueness check against index
- Field overrides via `--field key=value`
- Missing template fallback (minimal note with warning)
- Post-create re-index via `IndexFile`
- Validation before writing

**Out of scope:**
- Modifying P2e's `plan.CreateNote` (stays minimal for plan files)
- Go template engine (SRS uses `<%=var%>`, not `{{.var}}`)

## Template Processing

### Function

```go
// Process loads a template, substitutes variables, applies field overrides,
// and returns the complete note content ready to write.
func Process(cfg ProcessConfig) (*ProcessResult, error)

type ProcessConfig struct {
    VaultPath    string
    Path         string            // vault-relative path for the new note
    Type         string            // note type (must be in registry)
    Fields       map[string]string // --field overrides (key=value)
    Body         string            // --body override (replaces template body)
    TemplatePath string            // resolved template file path from config
}

type ProcessResult struct {
    Content  []byte   // complete file content (frontmatter + body)
    ID       string   // generated or provided ID
    Path     string   // vault-relative path
    Warnings []string // e.g., "unrecognized variable: <%=foo%>"
}
```

### Algorithm

1. **Load template** — read `TemplatePath` from disk. If file doesn't exist, use minimal fallback (frontmatter with core + required fields, empty body). Add warning.
2. **Build variable map:**
   - `<%=id%>` → from `--field id=...` or generated via `GenerateID`
   - `<%=type%>` → from `--type` flag
   - `<%=title%>` → from `--field title=...` or derived from filename (spaces→hyphens removed, title-cased)
   - `<%=created%>` → current date ISO 8601 (`YYYY-MM-DD`)
   - `<%=updated%>` → current date ISO 8601
   - `<%=date%>` → current date `YYYY-MM-DD`
   - `<%=datetime%>` → current datetime `YYYY-MM-DDTHH:MM:SS`
   - `<%=path%>` → vault-relative path
3. **Substitute variables** — replace all `<%=var%>` occurrences. Unrecognized variables left as-is with warning.
4. **Parse frontmatter** — extract YAML from the substituted template
5. **Apply field overrides** — `--field key=value` overrides frontmatter values
6. **Ensure core fields** — `id`, `type`, `created`, `vm_updated` always set even if template omits them
7. **Apply body override** — if `--body` provided, replace everything after frontmatter
8. **Serialize** — rebuild `---\nYAML\n---\nbody\n`
9. **Return** content + ID + warnings

### Variable Substitution

Uses simple string replacement with `<%=variable%>` delimiters. This avoids collision with:
- Obsidian Templater (`{{}}` and `${}`)
- Standard Markdown/HTML

```go
var templateVarRe = regexp.MustCompile(`<%=(\w+)%>`)

func substituteVars(content string, vars map[string]string) (string, []string) {
    var warnings []string
    result := templateVarRe.ReplaceAllStringFunc(content, func(match string) string {
        key := match[3 : len(match)-2] // extract between <%= and %>
        if val, ok := vars[key]; ok {
            return val
        }
        warnings = append(warnings, fmt.Sprintf("unrecognized variable: %s", match))
        return match // leave as-is
    })
    return result, warnings
}
```

## ID Generation

```go
// GenerateID creates an ID from the file path and type.
// Format: {type}-{slug} where slug is filename without .md, lowercased, spaces→hyphens.
func GenerateID(path, noteType string) string

// CheckIDUnique verifies the ID doesn't already exist in the index.
func CheckIDUnique(db *index.DB, id string) error
```

Examples:
- `projects/payment-retries.md` + type `project` → `project-payment-retries`
- `concepts/Working Memory.md` + type `concept` → `concept-working-memory`

Uniqueness check queries `SELECT id FROM notes WHERE id = ?`. If found, returns error with suggestion to use `--field id=custom-id`.

## Template Loading

Templates are at the path specified in `config.Types[type].Template` (e.g., `templates/project.md`), relative to vault root.

### Missing Template Fallback

If the template file doesn't exist:
1. Generate minimal note with frontmatter containing core fields (`id`, `type`, `created`, `vm_updated`) plus required fields for the type (from registry, set to empty strings)
2. Empty body
3. Emit warning: "template not found: {path}, using minimal fallback"

## CLI Command

```
vaultmind note create <path> --type <type> [--field key=value ...] [--body <text>] [--commit] [--json]
```

- `<path>` — vault-relative path for the new note
- `--type` — required, must be in registry
- `--field` — repeatable, overrides template frontmatter values
- `--body` — replaces template body text
- `--commit` — stage and commit the new file
- `--json` — JSON envelope output

### JSON Response

```json
{
  "path": "projects/payment-retries.md",
  "id": "project-payment-retries",
  "type": "project",
  "created": true,
  "write_hash": "sha256:...",
  "warnings": []
}
```

### Validation Before Writing

1. Path doesn't already exist on disk → `path_exists` error
2. Path stays within vault boundary → `path_traversal` error
3. Type is in registry → `unknown_type` error
4. ID is unique in index → `duplicate_id` error
5. Required fields for type are present in final frontmatter → `missing_required_field` error

## Testing Strategy

### Template processing tests
- All 8 variables substituted correctly
- Unrecognized variable left as-is with warning
- Field overrides replace template values
- Body override replaces template body
- Core fields always present even if template omits them
- Missing template → minimal fallback with warning

### ID generation tests
- Type-slug format from various paths
- Spaces → hyphens
- Nested paths (only filename used for slug)
- Uniqueness check passes / fails

### Integration tests
- Full create: template → process → write → verify file
- Create with `--commit` in temp git repo
- Path exists → error
- Unknown type → error
- Post-create re-index: note immediately queryable

### Coverage target
85%+ for `internal/template/` package.

## Design Decisions

### DD-1: Separate template package

**Choice:** `internal/template/` is new and independent from `plan.CreateNote`.

**Rationale:** The plan executor's `CreateNote` takes explicit frontmatter/body from JSON — no template processing needed. The CLI command needs template loading, variable substitution, ID generation — different concerns. Keeping them separate avoids bloating the plan executor.

### DD-2: `<%=var%>` substitution, not Go templates

**Choice:** Simple regex-based string replacement with `<%=variable%>` delimiters.

**Rationale:** SRS specifies this delimiter to avoid collision with Obsidian Templater. Go's `text/template` engine would add complexity and a different syntax. Simple replacement is sufficient for 8 known variables.

### DD-3: ID uniqueness check against index

**Choice:** Query the SQLite index before creating the note.

**Rationale:** Duplicate IDs cause resolution ambiguity. Catching duplicates at creation time is better than discovering them during `frontmatter validate`. The index must be current — the command should warn if index appears stale.

## File Inventory

| File | Purpose |
|------|---------|
| `internal/template/process.go` | `Process`, `ProcessConfig`, `ProcessResult`, `substituteVars` |
| `internal/template/id.go` | `GenerateID`, `CheckIDUnique` |
| `internal/template/process_test.go` | Template processing + substitution tests |
| `internal/template/id_test.go` | ID generation + uniqueness tests |
| `cmd/note_create.go` | `note create` subcommand |
| `internal/config/commands/note_create_config.go` | Config registration |
