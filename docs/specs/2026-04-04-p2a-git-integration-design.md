# P2a: Git Integration — Design Spec

> Phase 2 sub-project A. Foundation for all mutation commands.
>
> SRS references: [07-git-model.md](../../srs/07-git-model.md), [14-safety-model.md](../../srs/14-safety-model.md), [18-config-spec.md](../../srs/18-config-spec.md), [09-response-shapes.md](../../srs/09-response-shapes.md)

## Goal

Detect git repository state, enforce a configurable policy matrix before mutations, and provide staging + commit capabilities. Expose repo state via a `git status` command. This package becomes the foundation that all P2b mutation commands import.

## Scope

**In scope:**
- `internal/git/` package: state detection, policy evaluation, staging, committing
- `GoGitDetector` using go-git (pure Go, no external `git` binary dependency)
- `PolicyChecker` with data-driven matrix matching the SRS
- `Committer` for staging files and creating commits
- `git status` CLI command with JSON envelope output
- Query-layer function `query.RunGitStatus`
- Config registration for `app.git.status.*` keys

**Out of scope:**
- Branch management, remotes, push/pull/fetch
- Mutation commands themselves (P2b)
- Plan file execution (P2e)
- `--commit` flag on mutation commands (wired in P2b, uses `Committer` from this package)

## Data Types

```go
package git

// RepoState captures git repository state at a point in time.
type RepoState struct {
    RepoDetected     bool
    Branch           string
    Detached         bool
    MergeInProgress  bool
    RebaseInProgress bool
    WorkingTreeClean bool
    StagedFiles      []string
    UnstagedFiles    []string
    UntrackedFiles   []string
}

// OperationType classifies the caller's intended action.
type OperationType int

const (
    OpRead        OperationType = iota // query commands
    OpDryRun                           // --dry-run / --diff
    OpWrite                            // apply mutation to disk
    OpWriteCommit                      // apply + git commit
)

// PolicyDecision is the outcome of a single rule evaluation.
type PolicyDecision int

const (
    Allow  PolicyDecision = iota
    Warn
    Refuse
)

// PolicyResult is the aggregate outcome of a policy check.
type PolicyResult struct {
    Decision PolicyDecision // strictest across all triggered rules
    Reasons  []PolicyReason // all triggered rules, including warnings when refused for another rule
}

// PolicyReason describes one triggered policy rule.
type PolicyReason struct {
    Rule    string // stable identifier: "dirty_target", "detached_head", etc.
    Message string // human-readable explanation
}
```

## Interfaces and Components

```go
// RepoStateDetector abstracts git state detection (DI for testing).
type RepoStateDetector interface {
    Detect(vaultPath string) (RepoState, error)
}

// GoGitDetector implements RepoStateDetector using go-git.
type GoGitDetector struct{}

func (d *GoGitDetector) Detect(vaultPath string) (RepoState, error)

// PolicyChecker evaluates the git policy matrix.
type PolicyChecker struct {
    overrides map[string]PolicyDecision // from config
}

// NewPolicyChecker creates a checker from vault.GitPolicyConfig.
// Returns error if config contains invalid policy values.
func NewPolicyChecker(cfg vault.GitPolicyConfig) (*PolicyChecker, error)

// Check evaluates all policy rules for the given state and operation.
// targetPath identifies the file being mutated (empty for read-only).
func (pc *PolicyChecker) Check(state RepoState, op OperationType, targetPath string) PolicyResult

// Committer handles git staging and committing via go-git.
type Committer struct{}

// CommitFiles stages the given paths and creates a commit.
// Returns the commit SHA on success.
func (c *Committer) CommitFiles(vaultPath string, paths []string, message string) (string, error)
```

## Policy Matrix

Default matrix (matches SRS 07-git-model.md exactly):

| Rule | Condition | Read | DryRun | Write | WriteCommit |
|------|-----------|------|--------|-------|-------------|
| `dirty_unrelated` | Unstaged/staged files exist, target not among them | Allow | Allow | Warn | Warn |
| `dirty_target` | Target in StagedFiles or UnstagedFiles | Allow | Allow | Refuse | Refuse |
| `detached_head` | `Detached == true` | Allow | Allow | Warn | Refuse |
| `merge_in_progress` | `MergeInProgress \|\| RebaseInProgress` | Allow | Allow | Refuse | Refuse |
| `no_repo` | `RepoDetected == false` | Warn | Warn | Warn | Refuse |

### Override mechanics

Config values (`allow`, `warn`, `refuse`) from `git.policy.*` replace the **Write** column for that rule. WriteCommit inherits from Write with two guards:

- `detached_head`: WriteCommit always Refuse (cannot commit to detached HEAD)
- `no_repo`: WriteCommit always Refuse (cannot commit without a repo)

These guards are not overridable.

### Evaluation

All rules are evaluated. The strictest decision wins (Refuse > Warn > Allow). All triggered reasons are collected in `PolicyResult.Reasons` so warnings can be surfaced in the JSON envelope even when the overall decision is Refuse due to a different rule.

## go-git Integration

### GoGitDetector.Detect

1. `git.PlainOpenWithOptions(vaultPath, &git.PlainOpenOptions{DetectDotGit: true})` — walks up to find `.git`
2. If open fails: return `RepoState{RepoDetected: false}`
3. `repo.Head()` — extract branch name and detached state
4. Check merge in progress: test for `.git/MERGE_HEAD` file
5. Check rebase in progress: test for `.git/rebase-merge/` or `.git/rebase-apply/` directory
6. `worktree.Status()` — iterate entries, classify into staged/unstaged/untracked based on staging and worktree status codes

### Committer.CommitFiles

1. Open repo via `git.PlainOpen`
2. Get worktree
3. `worktree.Add(path)` for each file in `paths`
4. `worktree.Commit(message, &git.CommitOptions{Author: sigFromConfig})` — read author from repo git config
5. Return hex-encoded commit SHA

### Performance note

`worktree.Status()` scans the full working tree. Acceptable for single-command invocations. If profiling shows issues on large vaults, scope detection to the target file's directory in a future optimization.

## `git status` Command

### File structure

- `cmd/git.go` — parent command (`vaultmind git`)
- `cmd/git_status.go` — subcommand (`vaultmind git status`)
- `internal/config/commands/git_status_config.go` — config registration

### Wiring

```
cobra command → cmdutil.OpenVaultDB (for vault path) → GoGitDetector.Detect → query.RunGitStatus → envelope.OK → JSON output
```

`query.RunGitStatus(detector RepoStateDetector, vaultPath string)` returns the SRS response shape:

```json
{
  "repo_detected": true,
  "branch": "main",
  "detached": false,
  "merge_in_progress": false,
  "rebase_in_progress": false,
  "working_tree_clean": false,
  "staged_files": [],
  "unstaged_files": ["projects/payment-retries.md"],
  "untracked_files": ["scratch/temp.md"]
}
```

### Human-readable output

When `--json` is off, print a concise summary:

```
Branch:  main
Status:  dirty (2 unstaged, 1 untracked)
Merge:   none
```

### Command size

`cmd/git_status.go` stays under 30 lines. Logic lives in `query.RunGitStatus` and `internal/git/`.

## Testing Strategy

### Unit tests (no git repo)

- **PolicyChecker.Check** — table-driven tests covering every cell of the matrix (~20-25 cases). Feed `RepoState` struct literals + `OperationType`, assert `PolicyResult.Decision` and `PolicyResult.Reasons`.
- **Config override tests** — verify overrides replace Write column defaults. Verify detached_head/no_repo commit guards are not overridable.
- **NewPolicyChecker** — valid config parses, invalid values (e.g. `"block"`) return error.
- **Edge cases** — target path in both staged and unstaged, multiple rules triggering simultaneously, empty target path for read ops.

### Integration tests (real temp repos)

- **GoGitDetector** — create repos in `t.TempDir()` via go-git, manipulate into each state:
  - Clean repo
  - Dirty with untracked files
  - Dirty with unstaged changes
  - Dirty with staged changes
  - Detached HEAD
  - Mid-merge (create MERGE_HEAD file)
  - Mid-rebase (create rebase-merge directory)
  - Not a git repo (plain directory)
- **Committer** — create repo with initial commit, write a file, call `CommitFiles`, verify commit in log with correct message and only expected files.

### Command tests

- `git status` command — invoke via cobra, assert JSON envelope matches expected shape. Test with a real temp repo.

### Coverage target

85%+ for `internal/git/` package.

## Design Decisions

### DD-1: go-git over shelling out to `git`

**Choice:** Use the go-git library for all git operations.

**Alternatives considered:** Shelling out to the `git` binary on PATH.

**Rationale:** Pure Go — no external dependency on a specific `git` binary version. Directly testable without mocking command execution. VaultMind already requires Go to build; go-git adds no new toolchain dependency. Avoids platform-specific command parsing.

**Trade-off:** go-git adds a significant transitive dependency tree. Must verify license compatibility via `task check:license:source` after `go get`.

### DD-2: Single `internal/git/` package

**Choice:** All git concerns (detection, policy, commit) in one package.

**Alternatives considered:** Split into `internal/git/` (detection) and `internal/mutation/` (policy + orchestration).

**Rationale:** Policy evaluation is tightly coupled to git state — splitting prematurely adds indirection. The mutation engine (P2b) imports `internal/git/` and orchestrates the full write workflow. If the package grows too large, it can be split later with clear seams.

### DD-3: Interface-based DI for state detection

**Choice:** `RepoStateDetector` interface with `GoGitDetector` implementation.

**Alternatives considered:** Concrete struct only; test with real repos everywhere.

**Rationale:** Follows ADR-003 (dependency injection). Policy matrix logic is pure decision-making — trivially testable with `RepoState` struct literals. go-git integration gets focused integration tests. Keeps unit test suite fast.

### DD-4: Policy as data (lookup table)

**Choice:** Matrix encoded as rules mapping `(condition, operation_type)` to `PolicyDecision`, populated from config defaults + overrides.

**Alternatives considered:** Nested if/switch statements.

**Rationale:** Data-driven approach makes the matrix auditable, configurable, and easy to extend. Adding a new state or override is a config change, not a code change. Table-driven tests map 1:1 to matrix cells.

### DD-5: Follow SRS policy matrix exactly

**Choice:** Dirty-unrelated allows commit with warning. No additional restrictions beyond SRS.

**Alternatives considered:** Require `--force` to commit when any files are dirty.

**Rationale:** The SRS policy is well-thought-out and already configurable via `git.policy.*` overrides. Users who want stricter behavior can set `dirty_unrelated: refuse` in config.

## File Inventory

| File | Purpose |
|------|---------|
| `internal/git/state.go` | `RepoState`, `RepoStateDetector` interface, `GoGitDetector` |
| `internal/git/policy.go` | `PolicyChecker`, `PolicyResult`, `PolicyReason`, matrix defaults |
| `internal/git/commit.go` | `Committer` |
| `internal/git/types.go` | `OperationType`, `PolicyDecision` constants |
| `internal/git/state_test.go` | Integration tests for `GoGitDetector` |
| `internal/git/policy_test.go` | Unit tests for `PolicyChecker` |
| `internal/git/commit_test.go` | Integration tests for `Committer` |
| `cmd/git.go` | Parent `git` command |
| `cmd/git_status.go` | `git status` subcommand |
| `internal/config/commands/git_status_config.go` | Config registration |
| `internal/query/git_status.go` | `RunGitStatus` query function |
| `internal/query/git_status_test.go` | Query function tests |
