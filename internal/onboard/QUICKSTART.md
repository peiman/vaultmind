# VaultMind — Quick Start

The 20% that gets an agent wired. For the whole guide (preflight, project read,
migration, diff-before-write), run `vaultmind init --print-instructions --full`.

## 1. Install

```bash
# Pure-Go, MiniLM embeddings — works everywhere, no native deps:
go install github.com/peiman/vaultmind@latest

# BGE-M3 quality (darwin-arm64 / linux-amd64) — prebuilt, libonnxruntime bundled,
# no build: download vaultmind_<version>_<os>_<arch>_ort.tar.gz from the GitHub
# release and run the binary inside.
```

## 2. Scaffold a vault

```bash
vaultmind init <vault>          # e.g. ./vaultmind-identity or "$HOME/.vaultmind/persona"
```

## 3. Wire the Claude Code hooks

```bash
vaultmind hooks install --vault <vault-path> .
```

`--vault` bakes `VAULTMIND_VAULT=<path>` into every wired hook — no script editing.

## 4. Env-var routing

`VAULTMIND_VAULT` is the simple default: it drives persona-load, per-turn recall,
and episode-write at once. A dual-vault adopter can route each concern
independently with a per-concern override (each falls back to `VAULTMIND_VAULT`,
so single-var setups are unchanged):

| Var | Drives | Falls back to |
|---|---|---|
| `VAULTMIND_VAULT` | persona + recall + episodes (one var, simplest) | `vaultmind-identity` |
| `VAULTMIND_RECALL_VAULT` | per-turn recall (UserPromptSubmit) + read-tracking | `VAULTMIND_VAULT` |
| `VAULTMIND_EPISODE_VAULT` | episode writes (SessionEnd) | `VAULTMIND_VAULT` |
| `LOAD_PERSONA_VAULT` | persona load at SessionStart | `VAULTMIND_VAULT` |
| `LOAD_PERSONA_RESEARCH_VAULT` | optional 2nd vault — `vaultmind self` only (memory/activation state: hot/recent note titles), NOT a content `ask`; auto-fires if its dir exists | `vaultmind-vault`; skipped if dir absent |

Set each one inline in the settings.json `command` string (`VAR="value" bash <script>`).

## 5. Index + embed

```bash
vaultmind index --vault <vault> --embed
```

`index --embed` is content-hash incremental: only new/changed notes embed, the
rest are skipped. Per-note live re-embed is fine for a few notes; batch one
`index --embed` after a burst to amortize the one-time BGE-M3 model load.

## 6. First ask

```bash
vaultmind ask "who am I" --vault <vault>   # see what the agent would see
```

## 7. Full guide

```bash
vaultmind init --print-instructions --full
```
