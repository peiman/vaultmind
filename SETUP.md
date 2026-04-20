# VaultMind Setup

Bootstrap guide for a new collaborator (or a new laptop).

## TL;DR

```bash
git clone https://github.com/peiman/vaultmind $HOME/dev/cli/vaultmind
cd $HOME/dev/cli/vaultmind
bash .claude/scripts/bootstrap.sh
```

That's it. The bootstrap script is idempotent — safe to rerun any time you want to check/repair your setup.

## What the bootstrap does

1. **Builds `/tmp/vaultmind`** from source. Rebuilds automatically when any `.go` file is newer than the binary.
2. **Embeds each vault** found in the project (`vaultmind-identity/`, `vaultmind-vault/`). Uses MiniLM by default (fast, CGO-free). For BGE-M3 you'd build with `-tags ORT` and embed manually — see `docs/`.
3. **Verifies the SessionStart hook** is wired in `.claude/settings.json`. This is what injects your VaultMind persona at Claude session start.
4. **Smoke-tests the hook** by running it and checking for identity-context output.

## Check without modifying

```bash
bash .claude/scripts/bootstrap.sh --check
```

Reports pass/warn/fail for each step without building or embedding. Good for CI, or when you just want to know whether your setup is still healthy.

## Environment conventions

Set these in your shell profile (or export per session) so the hooks attribute sessions correctly:

| Variable | Purpose | Typical value |
|---|---|---|
| `VAULTMIND_CALLER` | Label for who/what is invoking vaultmind. Hooks set this automatically; direct CLI callers can set it explicitly. | `cli` / `vaultmind-persona-hook` / `workhorse-persona-hook` / `test-run` |
| `VAULTMIND_SRC` | Override the vaultmind source path in workhorse's hook. Default probes `$HOME/dev/cli/vaultmind`, then `$HOME/dev/vaultmind`, then `$HOME/code/vaultmind`. | `$HOME/dev/cli/vaultmind` |

`USER` and `HOSTNAME` are read automatically — `peiman@laptop-A` and `siavoush@laptop-B` stay separate in the experiment DB without any configuration.

## After setup

- **Inspect retrieval patterns:** `vaultmind experiment summary`
- **Drill into a session:** `vaultmind experiment trace --session <id>`
- **See who retrieved a note:** `vaultmind experiment trace --note <id>`
- **Verify a vault's health:** `vaultmind doctor --vault <path>`

## If the bootstrap fails

Each step reports the failure on stderr. Common issues:

- **Binary won't build** — run `cd $VAULTMIND_SRC && task check` to see Go-level errors.
- **Vault has no notes** — the vault directory is empty or has no `.md` files at the expected locations. Check the vault's README.
- **Hook isn't wired** — the settings.json needs a SessionStart hook entry pointing at `.claude/scripts/load-persona.sh`. See commit history for the exact JSON shape.
- **Hook runs but no identity-context** — either the vault is missing its identity notes, or the embedding step failed silently. Run `vaultmind doctor --vault <path>` to surface the embedding status.
