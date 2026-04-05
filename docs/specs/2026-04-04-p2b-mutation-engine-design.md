# P2b: Mutation Engine — Design Spec

> Phase 2 sub-project B. Frontmatter mutation commands with atomic writes, validation, and git integration.
>
> SRS references: [06-mutation-model.md](../srs/06-mutation-model.md), [13-validation-rules.md](../srs/13-validation-rules.md), [09-response-shapes.md](../srs/09-response-shapes.md), [07-git-model.md](../srs/07-git-model.md), [08-agent-contract.md](../srs/08-agent-contract.md)

## Goal

Implement frontmatter mutation commands (`set`, `unset`, `merge`, `normalize`) with a unified pipeline that enforces the SRS mutation workflow: read → compute → validate → diff → hash-check → atomic write → re-index → optional commit.

## Scope

**In scope:**
- `internal/mutation/` package: unified `Mutator` pipeline, YAML writer (yaml.Node round-tripping), byte-level splice, normalize operations, pre-mutation validation
- Four CLI commands: `frontmatter set`, `frontmatter unset`, `frontmatter merge`, `frontmatter normalize`
- `--dry-run`, `--diff`, `--commit`, `--allow-extra`, `--strip-time` flags
- Conflict detection via SHA-256 hash comparison
- Git policy enforcement via `internal/git/` (P2a)
- Post-write re-indexing of affected file
- Structured JSON responses matching SRS response shapes

**Out of scope:**
- Generated Dataview regions (P2d)
- Plan file parsing and `apply` command (P2e)
- Incremental indexing (P2c)
- `note create` (Phase 3)

## Data Types

```go
package mutation

// OpType identifies the mutation operation.
type OpType int

const (
    OpSet       OpType = iota // set a single key=value
    OpUnset                   // remove a key
    OpMerge                   // merge multiple key=value pairs
    OpNormalize               // reformat frontmatter
)

// MutationRequest describes what to mutate.
type MutationRequest struct {
    Op         OpType
    Target     string                 // id, path, or alias
    Key        string                 // for set/unset
    Value      interface{}            // for set
    Fields     map[string]interface{} // for merge
    DryRun     bool
    Diff       bool
    Commit     bool
    AllowExtra bool
    Force      bool
    StripTime  bool                   // normalize: force date-only
}

// MutationResult is the JSON response for all mutation commands.
type MutationResult struct {
    Path            string      `json:"path"`
    ID              string      `json:"id"`
    Operation       string      `json:"operation"`
    Key             string      `json:"key,omitempty"`
    OldValue        interface{} `json:"old_value,omitempty"`
    NewValue        interface{} `json:"new_value,omitempty"`
    DryRun          bool        `json:"dry_run"`
    Diff            string      `json:"diff,omitempty"`
    WriteHash       string      `json:"write_hash,omitempty"`
    Git             GitInfo     `json:"git"`
    ReindexRequired bool        `json:"reindex_required"`
}

// GitInfo reports git state in mutation responses.
type GitInfo struct {
    RepoDetected     bool   `json:"repo_detected"`
    WorkingTreeClean bool   `json:"working_tree_clean"`
    TargetFileClean  bool   `json:"target_file_clean"`
    CommitSHA        string `json:"commit_sha,omitempty"`
}
```

## Mutator Pipeline

```go
// Mutator orchestrates the 7-step mutation workflow.
type Mutator struct {
    VaultPath string
    Detector  git.RepoStateDetector
    Checker   *git.PolicyChecker
    Committer *git.Committer
    Registry  *schema.Registry
    Indexer   *index.Indexer
}

// Run executes a mutation request through the full pipeline.
func (m *Mutator) Run(req MutationRequest) (*MutationResult, error)
```

### The 7 Steps

1. **Resolve target** — use `graph.Resolver` to find the note by id, path, or alias. Return `unresolved_target` or `ambiguous_target` error if resolution fails.

2. **Read file** — read raw bytes from disk, compute `readHash = SHA-256(raw)`. Parse frontmatter into `*yaml.Node` tree via `ParseFrontmatterNode`. Record `bodyOffset` (byte position after closing `---`). Detect line ending convention and trailing newline state.

3. **Compute change** — dispatch to operation-specific logic:
   - `applySet(node, key, value)` — find or insert key in YAML node tree
   - `applyUnset(node, key)` — remove key from node tree
   - `applyMerge(node, fields)` — apply multiple set operations
   - `applyNormalize(node, stripTime)` — sort keys, fix scalar lists, normalize dates, snake_case keys

4. **Validate** — check the resulting frontmatter against schema:
   - `id` and `type` not modified → `immutable_field`
   - Key allowed by type schema (or `--allow-extra`) → `unknown_key`
   - Required fields still present after unset → `missing_required_field`
   - Valid status value if status changed → `invalid_status`
   - Note is domain note (for set/unset/merge) → `not_domain_note`
   - Git policy check via `PolicyChecker.Check(state, opType, targetPath)`

5. **Generate diff** — serialize modified `yaml.Node` back to YAML bytes via `SerializeFrontmatter`. Splice into file bytes via `SpliceFile` (replace frontmatter, keep body untouched). Generate unified diff against original bytes. If `--dry-run`, return result with diff string, skip steps 6-7.

6. **Write atomically** — re-read file, compute hash, compare to `readHash`. If mismatch → `conflict` error. Write spliced bytes to temp file in same directory, `os.Rename` over original. Compute `writeHash = SHA-256(new bytes)`.

7. **Post-write** — re-index the affected file via `Indexer`. If `--commit`, stage and commit via `Committer` with message format: `vaultmind: frontmatter {op} {id} {key}={value} — {summary}`.

Each step returns structured errors mapping to SRS error codes. The pipeline stops at the first error — no partial writes.

## Frontmatter Writer (`yamlwriter.go`)

The most delicate component — responsible for faithful YAML round-tripping.

```go
// ParseFrontmatterNode parses raw file bytes into a yaml.Node tree
// and returns the body byte offset (position after closing ---).
func ParseFrontmatterNode(raw []byte) (node *yaml.Node, bodyOffset int, err error)

// SetKey sets or inserts a key in the yaml.Node mapping.
// Preserves position of existing keys. New keys are appended.
func SetKey(doc *yaml.Node, key string, value interface{}) error

// UnsetKey removes a key from the yaml.Node mapping.
func UnsetKey(doc *yaml.Node, key string) error

// SerializeFrontmatter marshals the yaml.Node back to YAML bytes,
// wrapped in --- delimiters, using the detected line ending convention.
func SerializeFrontmatter(doc *yaml.Node, lineEnding string) ([]byte, error)

// SpliceFile replaces the frontmatter section in raw bytes with new YAML,
// preserving body bytes untouched and normalizing the gap
// (exactly one blank line between frontmatter and body).
func SpliceFile(original []byte, newFrontmatter []byte, bodyOffset int) []byte

// DetectLineEnding returns "\r\n" if CRLF detected, "\n" otherwise.
func DetectLineEnding(raw []byte) string

// DetectTrailingNewline returns true if the file ends with a newline.
func DetectTrailingNewline(raw []byte) bool
```

**Key behaviors:**
- `yaml.v3`'s `yaml.Node` preserves key order, scalar styles (quoted vs bare), and comments
- `SetKey` walks the mapping node's `Content` slice (alternating key/value nodes) to find existing keys. For new keys, appends at end.
- `SerializeFrontmatter` marshals via `yaml.NewEncoder` writing to a buffer, then applies line ending normalization
- `SpliceFile` does: `newFrontmatter + "\n" + original[bodyOffset:]`, preserving trailing newline state

## Normalize Operations (`normalize.go`)

```go
// SortKeys reorders yaml.Node mapping content into canonical order:
// id, type, status, title, aliases, tags, created, updated, then remaining alphabetically.
func SortKeys(doc *yaml.Node)

// ScalarToList converts scalar aliases/tags values to single-element lists.
func ScalarToList(doc *yaml.Node, key string) bool

// NormalizeDates converts date fields to YYYY-MM-DD when time is T00:00:00.
// With stripTime=true, forces all datetimes to date-only.
func NormalizeDates(doc *yaml.Node, stripTime bool)

// SnakeCaseKeys converts non-snake_case keys to snake_case.
// Returns the list of renames performed.
func SnakeCaseKeys(doc *yaml.Node) []KeyRename

// KeyRename records a key that was renamed during normalization.
type KeyRename struct {
    OldKey string
    NewKey string
}
```

**Canonical key order:** `id`, `type`, `status`, `title`, `aliases`, `tags`, `created`, `updated`, then remaining keys alphabetically.

Each normalize sub-operation is individually callable — the `applyNormalize` function in the pipeline calls them all, but future flags could skip individual operations.

## Pre-Mutation Validation (`validate.go`)

```go
// ValidateMutation checks the mutation against schema and invariants.
// Returns a structured error with the appropriate SRS error code, or nil.
func ValidateMutation(req MutationRequest, note ParsedNoteInfo, reg *schema.Registry) error
```

`ParsedNoteInfo` is a lightweight struct with the fields needed for validation (id, type, isDomain, existing frontmatter keys) — avoids importing the full parser.

| Check | Error code | Applies to |
|---|---|---|
| Target doesn't resolve | `unresolved_target` | All |
| Target resolves ambiguously | `ambiguous_target` | All |
| Note is unstructured | `not_domain_note` | set, unset, merge |
| Modifying `id` or `type` | `immutable_field` | set, merge |
| Key not in type schema | `unknown_key` | set, merge (unless `--allow-extra`) |
| Required field being removed | `missing_required_field` | unset |
| Invalid status value | `invalid_status` | set, merge (when key is `status`) |
| File changed on disk since read | `conflict` | write step (in mutator, not here) |
| Git policy refuses | per policy rule | write/commit steps (in mutator, not here) |

## CLI Commands

Four new subcommands under the existing `frontmatter` parent:

```
vaultmind frontmatter set <target> <key> <value> [--dry-run] [--diff] [--commit] [--allow-extra] [--json]
vaultmind frontmatter unset <target> <key> [--dry-run] [--diff] [--commit] [--json]
vaultmind frontmatter merge <target> --file <yaml-file> [--dry-run] [--diff] [--commit] [--allow-extra] [--json]
vaultmind frontmatter normalize [<path-or-glob>] [--dry-run] [--diff] [--commit] [--strip-time] [--json]
```

**Command files** (each ≤30 lines):
- `cmd/frontmatter_set.go`
- `cmd/frontmatter_unset.go`
- `cmd/frontmatter_merge.go`
- `cmd/frontmatter_normalize.go`

**Wiring pattern:** Each command constructs a `MutationRequest`, creates a `Mutator` (from `cmdutil.OpenVaultDB` + git components), calls `Mutator.Run`, and writes the response via `envelope.OK` or returns the error.

**Normalize multi-file handling:** The `normalize` command accepts a path or glob, potentially matching multiple files. The command layer resolves the glob, then loops calling `Mutator.Run` per file. Each file gets its own pipeline execution (read, validate, write). The JSON response for multi-file normalize is an array of `MutationResult` objects. If any file fails, the error is reported and remaining files are still processed (best-effort, not atomic across files).

**Config registration:** `internal/config/commands/frontmatter_mutation_config.go` registers metadata for all four subcommands with their flags. Shared flags (`--dry-run`, `--diff`, `--commit`) use the `app.frontmatter` prefix.

**Envelope output:** JSON responses use the existing `envelope.OK` wrapper. Mutation responses include `write_hash` and `reindex_required` per the agent contract. Warnings from git policy checks are added via `envelope.AddWarning`.

## Git Integration

Uses `internal/git/` from P2a:

- **Before write:** `PolicyChecker.Check(state, git.OpWrite, targetPath)` — if Refuse, return error with policy reasons. If Warn, add warnings to envelope.
- **Before commit:** `PolicyChecker.Check(state, git.OpWriteCommit, targetPath)` — same logic.
- **Commit:** `Committer.CommitFiles(vaultPath, []string{targetRelPath}, message)` — stages only the mutated file.
- **Commit message format:** `vaultmind: frontmatter {op} {id} {key}={value} — {summary}` (per SRS 07-git-model.md).
- **GitInfo in response:** Always populated with current repo state, regardless of whether `--commit` is used.

## Testing Strategy

**TDD throughout — every function gets a failing test first, then implementation.**

### Unit tests (no disk, no git)

- **YAML writer functions** (most critical):
  - `ParseFrontmatterNode` — round-trip: parse then serialize, compare byte-for-byte
  - `SetKey` — existing key update, new key insert, nested value, list value
  - `UnsetKey` — existing key removal, non-existent key (no-op)
  - `SpliceFile` — body preserved byte-for-byte, gap normalized, trailing newline preserved
  - `DetectLineEnding` — LF file, CRLF file, mixed (first wins)
  - `DetectTrailingNewline` — with/without trailing newline
  - `SortKeys` — canonical order verified
  - `ScalarToList` — string becomes `[string]`, already-list is no-op
  - `NormalizeDates` — `2026-04-04T00:00:00` → `2026-04-04`, non-zero time preserved
  - `SnakeCaseKeys` — `camelCase` → `camel_case`, already-snake no-op, returns rename list

- **Validation logic** — struct-literal inputs:
  - Immutable field check (id, type)
  - Unknown key rejection / allow-extra bypass
  - Required field removal detection
  - Invalid status detection
  - Domain note requirement

- **MutationResult construction** — verify JSON shape matches SRS

### Integration tests (temp directories)

- **Full pipeline** — create temp vault with config + notes, run `Mutator.Run`:
  - Set a field → verify file updated, diff correct, hash returned
  - Unset a field → verify field removed, required field check works
  - Merge fields → verify multiple fields updated atomically
  - Normalize → verify key order, scalar-to-list, date normalization
  - Dry-run → verify file untouched, diff returned
  - Conflict detection → modify file between read and write, expect `conflict` error
  - Git policy refuse → dirty target, expect error

- **Commit integration** — temp git repo, run mutation with `Commit: true`, verify commit message matches format

### Coverage target

90%+ for `internal/mutation/` package.

## Design Decisions

### DD-1: Unified mutation pipeline

**Choice:** Single `Mutator.Run(op)` with shared 7-step workflow.

**Alternatives considered:** Per-operation functions sharing helpers.

**Rationale:** The workflow is identical for all four operations — only the "compute change" step differs. A unified pipeline avoids duplicating the 7-step workflow four times and makes it easy to add new operations (e.g., generated region rendering in P2d).

### DD-2: `yaml.Node` tree manipulation

**Choice:** Parse frontmatter into `*yaml.Node`, modify the tree directly, marshal back.

**Alternatives considered:** `map[string]interface{}` with ordered key tracking.

**Rationale:** SRS requires "preserve YAML key order" and "minimize noisy diffs". `yaml.Node` preserves key order, comments, and scalar styles (quoted vs bare). The map approach loses comments and may normalize scalar styles, producing noisy diffs. The `yaml.Node` API is verbose but contained in a single writer module.

### DD-3: Byte-level splice

**Choice:** Replace frontmatter bytes only, keep body bytes untouched.

**Alternatives considered:** Re-serialize the entire file.

**Rationale:** SRS requires "preserve unrelated content byte-for-byte". Splicing the frontmatter section and keeping body bytes untouched is the only way to guarantee this. Re-serializing could alter trailing whitespace, line endings, or other body formatting.

### DD-4: New `internal/mutation/` package

**Choice:** Dedicated package for mutation logic, separate from `internal/query/`.

**Alternatives considered:** Extend `internal/query/` with mutation functions.

**Rationale:** Mutations write files, check git policy, do hash verification, and optionally commit — fundamentally different from read-only queries. Separating concerns keeps the architecture clean and architecture lint straightforward.

### DD-5: 90% coverage target

**Choice:** 90% coverage for `internal/mutation/`, above the project minimum of 85%.

**Rationale:** Mutation code touches user files. Bugs can corrupt vault content. Higher coverage provides stronger confidence in correctness, especially for the YAML round-tripping and byte-level splice logic.

## File Inventory

| File | Purpose |
|------|---------|
| `internal/mutation/types.go` | `OpType`, `MutationRequest`, `MutationResult`, `GitInfo`, `ParsedNoteInfo` |
| `internal/mutation/mutator.go` | `Mutator` struct, `Run` pipeline (7 steps) |
| `internal/mutation/yamlwriter.go` | `ParseFrontmatterNode`, `SetKey`, `UnsetKey`, `SerializeFrontmatter`, `SpliceFile`, line ending/trailing newline detection |
| `internal/mutation/normalize.go` | `SortKeys`, `ScalarToList`, `NormalizeDates`, `SnakeCaseKeys` |
| `internal/mutation/validate.go` | `ValidateMutation` — pre-mutation schema and invariant checks |
| `internal/mutation/types_test.go` | Type tests |
| `internal/mutation/yamlwriter_test.go` | YAML round-trip and splice tests |
| `internal/mutation/normalize_test.go` | Normalize operation tests |
| `internal/mutation/validate_test.go` | Validation logic tests |
| `internal/mutation/mutator_test.go` | Full pipeline integration tests |
| `cmd/frontmatter_set.go` | `frontmatter set` subcommand |
| `cmd/frontmatter_unset.go` | `frontmatter unset` subcommand |
| `cmd/frontmatter_merge.go` | `frontmatter merge` subcommand |
| `cmd/frontmatter_normalize.go` | `frontmatter normalize` subcommand |
| `internal/config/commands/frontmatter_mutation_config.go` | Config registration for set/unset/merge/normalize |
