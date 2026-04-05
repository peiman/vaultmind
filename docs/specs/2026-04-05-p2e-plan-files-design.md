# P2e: Plan Files — Design Spec

> Phase 2 sub-project E (final). Batch mutation execution via JSON plan files with rollback.
>
> SRS references: [10-plan-files.md](../srs/10-plan-files.md), [06-mutation-model.md](../srs/06-mutation-model.md), [09-response-shapes.md](../srs/09-response-shapes.md), [07-git-model.md](../srs/07-git-model.md)

## Goal

Implement the `apply` command that accepts a JSON plan file describing a batch of mutations and executes them in order with best-effort rollback on failure and optional single batch commit.

## Scope

**In scope:**
- `internal/plan/` package: parse, validate, execute, rollback
- `apply` command accepting plan file path or `-` for stdin
- 5 operation types: `frontmatter_set`, `frontmatter_unset`, `frontmatter_merge`, `generated_region_render`, `note_create`
- Minimal `note_create` (write frontmatter + body, no template rendering)
- In-memory rollback on failure (restore pre-operation file copies)
- Single batch commit with `--commit` (message from plan description)
- `--dry-run` and `--diff` flags
- JSON response matching SRS `apply` shape
- Post-execution re-index via `IndexFile`

**Out of scope:**
- Full `note create` CLI command with templates (Phase 3)
- Plan file generation (agents generate these)
- Interactive plan editing

## Plan File Format

```json
{
  "version": 1,
  "description": "Pause all billing projects",
  "operations": [
    {"op": "frontmatter_set", "target": "proj-payment-retries", "key": "status", "value": "paused"},
    {"op": "frontmatter_unset", "target": "proj-payment-retries", "key": "due"},
    {"op": "frontmatter_merge", "target": "proj-billing-dashboard", "fields": {"status": "paused", "owner_id": "person-alice"}},
    {"op": "generated_region_render", "target": "proj-payment-retries", "section_key": "related", "template": "related"},
    {"op": "note_create", "path": "decisions/pause-billing.md", "type": "decision", "frontmatter": {"title": "Pause billing", "status": "accepted"}, "body": "## Context\n\nPaused pending Q3 review.\n"}
  ]
}
```

## Types (`internal/plan/types.go`)

```go
// Plan is the parsed JSON plan file.
type Plan struct {
    Version     int         `json:"version"`
    Description string      `json:"description"`
    Operations  []Operation `json:"operations"`
}

// Operation is one step in a plan.
type Operation struct {
    Op          string                 `json:"op"`
    Target      string                 `json:"target,omitempty"`
    Key         string                 `json:"key,omitempty"`
    Value       interface{}            `json:"value,omitempty"`
    Fields      map[string]interface{} `json:"fields,omitempty"`
    SectionKey  string                 `json:"section_key,omitempty"`
    Template    string                 `json:"template,omitempty"`
    Path        string                 `json:"path,omitempty"`
    Type        string                 `json:"type,omitempty"`
    Frontmatter map[string]interface{} `json:"frontmatter,omitempty"`
    Body        string                 `json:"body,omitempty"`
}

// OpResult is the per-operation outcome.
type OpResult struct {
    Op        string  `json:"op"`
    Target    string  `json:"target,omitempty"`
    Path      string  `json:"path,omitempty"`
    ID        string  `json:"id,omitempty"`
    Status    string  `json:"status"`      // "ok", "error", "skipped"
    WriteHash string  `json:"write_hash,omitempty"`
    Error     *OpError `json:"error,omitempty"`
}

// OpError is a structured error for a failed operation.
type OpError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

// ApplyResult is the JSON response for the apply command.
type ApplyResult struct {
    PlanDescription     string     `json:"plan_description"`
    OperationsTotal     int        `json:"operations_total"`
    OperationsCompleted int        `json:"operations_completed"`
    Operations          []OpResult `json:"operations"`
    Committed           bool       `json:"committed"`
    CommitSHA           string     `json:"commit_sha,omitempty"`
}
```

## Plan Validation (`internal/plan/validate.go`)

```go
// ValidatePlan checks plan structure without executing anything.
// Returns a list of validation errors (empty if valid).
func ValidatePlan(plan Plan, reg *schema.Registry) []OpError
```

Validation checks (from SRS):

| Condition | Error code |
|---|---|
| `version` != 1 | `unsupported_version` |
| Unknown `op` value | `unknown_operation` |
| Missing required field for op | `missing_field` |
| `note_create` type not in registry | `unknown_type` |

Required fields per operation:
- `frontmatter_set`: `target`, `key`, `value`
- `frontmatter_unset`: `target`, `key`
- `frontmatter_merge`: `target`, `fields`
- `generated_region_render`: `target`, `section_key`, `template`
- `note_create`: `path`, `type`, `frontmatter`

Note: `target` resolution and `path_exists` are checked at execution time (not validation time), because `note_create` in an earlier operation can create a file referenced by a later operation.

## Executor (`internal/plan/executor.go`)

```go
// Executor runs a plan against a vault.
type Executor struct {
    VaultPath string
    Detector  git.RepoStateDetector
    Checker   *git.PolicyChecker
    Committer *git.Committer
    Registry  *schema.Registry
    Config    *vault.Config
}

// Apply executes a plan. Returns the result even on partial failure.
func (e *Executor) Apply(plan Plan, dryRun, diff, commit bool) (*ApplyResult, error)
```

### Execution Pipeline

1. **Validate plan structure** — call `ValidatePlan`. If errors, return immediately with all ops marked "skipped".

2. **Check git policy** — detect state, check policy for `OpWrite` (or `OpWriteCommit` if `--commit`). If Refuse, return error.

3. **Execute operations in order.** For each operation:
   a. **Backup** — read current file bytes into in-memory `backups` map (keyed by path). For `note_create`, record that the file didn't exist (rollback = delete).
   b. **Dispatch** to operation handler:
      - `frontmatter_set` → `mutation.Mutator.Run(MutationRequest{Op: OpSet, ...})`
      - `frontmatter_unset` → `mutation.Mutator.Run(MutationRequest{Op: OpUnset, ...})`
      - `frontmatter_merge` → `mutation.Mutator.Run(MutationRequest{Op: OpMerge, ...})`
      - `generated_region_render` → `marker.RenderRegion(RenderConfig{...})`
      - `note_create` → `createNote(op)` (internal handler)
   c. **Record result** — `OpResult{Status: "ok", WriteHash: ...}`
   d. **On failure** — record error, mark remaining ops as "skipped", trigger rollback, return.

4. **Rollback on failure** — iterate backwards through completed operations:
   - For files that existed before: write the backup bytes back (atomic write)
   - For files created by `note_create`: delete them
   - Rollback is best-effort — errors are logged but don't prevent further rollback

5. **Commit on success** — if `--commit` and all ops succeeded:
   - Collect all modified/created file paths
   - `Committer.CommitFiles(vaultPath, paths, message)`
   - Message: `vaultmind: apply — {plan.Description}`

6. **Re-index** — call `IndexFile` for each modified/created file.

### Dry-run Mode

When `--dry-run`:
- Operations are NOT executed
- For each operation, compute what would happen (resolve target, validate) but don't write
- Return results with status "ok" (indicating the operation would succeed) and diffs if `--diff`
- No rollback needed since nothing is written

## note_create Handler (`internal/plan/create.go`)

```go
func (e *Executor) createNote(op Operation) (*OpResult, error)
```

1. Validate: `filepath.Join(vaultPath, op.Path)` doesn't exist → `path_exists` error if it does
2. Validate: path stays within vault (path traversal check)
3. Validate: `op.Type` is in registry → `unknown_type` error if not
4. Build frontmatter: start with `op.Frontmatter`, inject `id` (from frontmatter if provided, else derived from filename), inject `type`
5. Serialize: `---\n{yaml}\n---\n{body}\n`
6. Create parent directories if needed (`os.MkdirAll`)
7. Write file
8. Return `OpResult{Status: "ok", Path: op.Path, ID: id, WriteHash: sha256}`

ID derivation when not provided in frontmatter: strip directory prefix, remove `.md` extension, prepend type. E.g., `decisions/pause-billing.md` with type `decision` → `decision-pause-billing`.

## CLI Command

```
vaultmind apply <plan-file | -> [--dry-run] [--diff] [--commit] [--json]
```

- `<plan-file>`: path to JSON plan file
- `-`: read plan from stdin (for agent piping)
- `cmd/apply.go` — thin command, reads plan JSON, delegates to `plan.Executor.Apply()`

### Human-readable Output

```
Applying plan: Pause all billing projects (4 operations)
  [1/4] frontmatter_set proj-payment-retries status=paused ... ok
  [2/4] frontmatter_merge proj-billing-dashboard ... ok
  [3/4] generated_region_render proj-payment-retries related ... ok
  [4/4] note_create decisions/pause-billing.md ... ok
Plan applied successfully (4/4 operations)
```

On failure:
```
  [2/4] frontmatter_merge proj-billing-dashboard ... ERROR: unknown_key
  [3/4] frontmatter_set ... skipped
  [4/4] note_create ... skipped
Rolling back 1 operation...
Plan failed: 1/4 operations completed, 1 rolled back
```

## Testing Strategy

### Unit tests

- **Plan parsing:** valid JSON, invalid JSON, empty operations array
- **Validation:** unsupported_version, unknown_operation, missing_field for each op type, unknown_type for note_create
- **note_create:** file created with correct frontmatter, path_exists error, unknown_type error, path traversal blocked, ID derivation

### Integration tests (temp vault)

- **Full execution:** 2-3 op plan, verify all files modified correctly
- **Rollback:** plan with intentional failure mid-way (e.g., second op targets nonexistent file), verify first op rolled back
- **Dry-run:** verify no files written, results returned
- **note_create in plan:** create note then set its frontmatter in same plan
- **Batch commit:** temp git repo, verify single commit with correct message and all files staged
- **Stdin input:** pass plan via `-` argument

### Coverage target

85%+ for `internal/plan/` package.

## Design Decisions

### DD-1: Minimal note_create

**Choice:** Write frontmatter + body directly, no template rendering.

**Rationale:** The full `note create` command with templates is Phase 3. The minimal version enables complete plan file support with ~50 lines of code. Template support can be added later without changing the plan file format.

### DD-2: In-memory rollback

**Choice:** Store pre-operation file bytes in a `map[string][]byte`.

**Rationale:** Plans typically have <20 operations. Keeping file backups in memory is negligible. The SRS explicitly says crash resilience isn't required ("if the process is killed mid-plan, partial writes may persist").

### DD-3: New `internal/plan/` package

**Choice:** Dedicated package, separate from mutation and marker.

**Rationale:** The executor orchestrates across mutation + marker + git. Putting it in either mutation or marker would create confusing dependencies. `plan` → `mutation` + `marker` + `git` is a clean dependency graph.

## File Inventory

| File | Purpose |
|------|---------|
| `internal/plan/types.go` | `Plan`, `Operation`, `OpResult`, `ApplyResult`, `OpError` |
| `internal/plan/validate.go` | `ValidatePlan` — structure validation |
| `internal/plan/executor.go` | `Executor`, `Apply` pipeline, rollback |
| `internal/plan/create.go` | `note_create` operation handler |
| `internal/plan/types_test.go` | Type tests |
| `internal/plan/validate_test.go` | Validation tests |
| `internal/plan/executor_test.go` | Execution + rollback integration tests |
| `internal/plan/create_test.go` | note_create tests |
| `cmd/apply.go` | `apply` CLI command |
| `internal/config/commands/apply_config.go` | Config registration |
