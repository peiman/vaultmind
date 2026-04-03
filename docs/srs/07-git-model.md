# Git Model

> See also: [mutation model](06-mutation-model.md), [safety](14-safety-model.md), [config](18-config-spec.md)

## Assumptions

VaultMind assumes the vault resides inside a Git repository. If no repo is detected, Git-aware features degrade gracefully: mutations still work but without safety checks, staging, or commits. A warning is emitted.

## Git Policy Matrix

| Repo State | Read | Write (dry-run/diff) | Write (apply) | Write (apply + commit) |
|------------|------|---------------------|---------------|----------------------|
| Clean working tree | Allow | Allow | Allow | Allow |
| Dirty — only unrelated files | Allow | Allow | Allow + warn | Allow + warn |
| Dirty — target file has unstaged changes | Allow | Allow | **Refuse** | **Refuse** |
| Dirty — target file has staged changes | Allow | Allow | **Refuse** | **Refuse** |
| Detached HEAD | Allow | Allow | Allow + warn | **Refuse** |
| Merge/rebase in progress | Allow | Allow | **Refuse** | **Refuse** |
| Not a Git repo | Allow + warn | Allow + warn | Allow + warn | **Refuse** |

### Configurable Overrides

The default policy can be relaxed per state in [config](18-config-spec.md):

```yaml
git:
  policy:
    dirty_unrelated: warn      # default: warn
    dirty_target: refuse       # default: refuse
    detached_head: warn        # default: warn (refuse for commit)
    merge_in_progress: refuse  # default: refuse
    no_repo: warn              # default: warn (refuse for commit)
```

Valid values: `refuse`, `warn`, `allow`.

## Commit Behavior

When `--commit` is passed:

1. Stage **only** the files modified by this operation
2. Generate commit message from structured change summary
3. Format: `vaultmind: {command} {target_id} — {summary}`
4. Example: `vaultmind: frontmatter set proj-payment-retries status=paused — updated status`
5. Commits are always single-operation. Batch commits not supported in v1.

Exception: `apply` command with `--commit` creates a single commit covering all plan operations, with message derived from the plan `description`.

## Git Boundaries

VaultMind wraps standard Git operations for its mutation workflows. It does **not** implement:

- Branch management
- Remote operations (push, pull, fetch)
- Merge/rebase
- Log browsing
