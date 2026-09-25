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

## 3. Wire the hooks

```bash
# Claude Code (.claude/settings.json)
vaultmind hooks install --vault <vault-path> --merge .

# Codex CLI (.codex/hooks.json)
vaultmind hooks install --vault <vault-path> --agent codex --merge .
```

`--vault` bakes `VAULTMIND_VAULT=<path>` into every wired hook — no script editing.

**Codex: approve the hooks, or nothing runs.** Codex skips unapproved hooks
*silently*: the agent starts with no memory and nothing tells you why. Tell the
person, plainly:

1. Open Codex in the project and **trust the project** when it asks.
2. Inside Codex, run **`/hooks`** and **trust the VaultMind hooks**.
3. **After every upgrade, do step 2 again**: a new or changed hook is skipped
   until it is approved.

Then confirm it rather than assume it:

```bash
vaultmind hooks status .   # lists each VaultMind hook Codex has not approved; fails until all are
```

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
| `VAULTMIND_PERSONA_MODE` | how the identity arrives at SessionStart: `served` (as text), `explore` (the agent is pointed at its vaults and looks), `alternate` (split by session, for comparison) | `served` |

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

## 8. Report what you find

You are the consumer whose real use makes this tool honest — the surest source of
fixes. When something surprises you, degrades silently, or makes you reach around
the tool, **file an issue: https://github.com/peiman/vaultmind/issues**. Concrete
field reports from adopters drive what gets fixed next — this onboarding keeps
improving because of them.
