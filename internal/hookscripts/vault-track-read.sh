#!/bin/bash
# PreToolUse hook on Read — when the agent reads a vault note, fire
# RecordNoteAccess via `vaultmind note get` AND inject a visible header
# naming the canonical retrieval command. Allow Read to proceed.
#
# This is flavor B of the read-bypass design discussion (2026-05-02):
# substitute-and-allow. The hook surfaces the right command name on
# every vault Read so the muscle memory shifts. Read still works,
# Edit on vault notes still works (since Read isn't blocked).
#
# Lifecycle history:
#   - Originally PostToolUse(Read) (commit f219f0e). Closed the
#     bookkeeping bypass (access tracking) but stayed silent — agent
#     never learned the canonical command.
#   - Now PreToolUse(Read) with additionalContext injection. Same
#     tracking, plus visibility. Probe-before-commit on flavor C.
#
# Flavor C (block-and-redirect) is preserved on disk in
# vault-block-read.sh + test-vault-block-read.sh, unwired, ready to
# enable if B's data shows visibility-only isn't sufficient to shift
# retrieval pattern.
#
# Mechanism: detect vault paths, run `vaultmind note get` synchronously
# (captures output to detect whether the path actually resolves to an
# indexed note — note get exits 0 even for unresolved ids, printing
# "No note found"). On resolution, emit JSON
# `hookSpecificOutput.additionalContext` to inject a header the agent
# sees. On non-resolution (episode files, frontmatter quirks, transient
# errors), silently allow Read — no misleading header.
#
# Output strategy: low-noise. The header is a single short line
# pointing at the canonical command. Exit 0 always — this hook is
# non-blocking by design.

set -uo pipefail

HOOK_INPUT=$(cat)

TOOL_NAME=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_name',''))" 2>/dev/null || echo "")
FILE_PATH=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")

if [ "$TOOL_NAME" != "Read" ] || [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Path pattern gate — fast-skip Reads on files that obviously aren't
# vault notes (the walk-up `.vaultmind/` check below is the real test
# but is filesystem-stat-per-ancestor; this glob filters early).
# Default `*/vaultmind-*/*.md` matches vaultmind-self conventions
# (vaultmind-identity, vaultmind-vault). Consumers with non-default
# vault dir names override per project — one project might use
# `*/their-vault/*.md`; another might use
# `*/vaultmind-knowledge/*.md`. Set inline in settings.json command,
# e.g. `VAULT_PATH_PATTERN="*/companion-vault/*.md" bash <script>`.
#
# Multi-vault: enable shopt extglob below so consumers needing more
# than one vault dir can use the `+(a|b)` extglob syntax — e.g.
# `VAULT_PATH_PATTERN="*/+(vault-a|another-vault)/*.md"`.
# Brace expansion (`*/{a,b}/*.md`) does NOT work from env vars (bash
# expands braces before parameter expansion); pipe alternation
# without extglob also doesn't work. Extglob does.
#
# Companion project 2026-05-07 HIGH-1 — silent-inert hook for non-default
# vault names was the dogfood-found regression.
shopt -s extglob
# Pattern precedence (issue #41.6): an explicit VAULT_PATH_PATTERN wins (the
# companion project extglob-alternation case). Otherwise, when VAULTMIND_VAULT is set
# (by `vaultmind hooks install --vault`), derive the pattern from the vault's
# basename so read-tracking actually fires for a consumer vault — bash `case`
# globs span `/`, so `*/<name>/*.md` matches notes in subdirectories too.
# Falls back to the vaultmind-* convention when neither is set.
if [ -n "${VAULT_PATH_PATTERN:-}" ]; then
  PATTERN="${VAULT_PATH_PATTERN:-}"
elif [ -n "${VAULTMIND_VAULT:-}" ]; then
  PATTERN="*/${VAULTMIND_VAULT##*/}/*.md"
else
  PATTERN="*/vaultmind-*/*.md"
fi
case "$FILE_PATH" in
  $PATTERN) ;;
  *) exit 0 ;;
esac

VAULT_ROOT=$(dirname "$FILE_PATH")
while [ "$VAULT_ROOT" != "/" ] && [ "$VAULT_ROOT" != "." ]; do
  if [ -d "$VAULT_ROOT/.vaultmind" ]; then
    break
  fi
  VAULT_ROOT=$(dirname "$VAULT_ROOT")
done
if [ ! -d "$VAULT_ROOT/.vaultmind" ]; then
  exit 0
fi

# Use PATH-installed vaultmind. /tmp/vaultmind is dev-loop only
# (load-persona.sh auto-rebuild target) — not a valid fallback for
# general use; users install via `task install`. Silently skip if
# not on PATH (PreToolUse hook must never block a Read).
if ! command -v vaultmind >/dev/null 2>&1; then
  exit 0
fi
VAULTMIND=$(command -v vaultmind)

REL_PATH="${FILE_PATH#"$VAULT_ROOT"/}"

# Sidecar log directory — captures invocations for verification.
LOG_DIR="${HOME}/.vaultmind/preread-track"
mkdir -p "$LOG_DIR" 2>/dev/null
TIMESTAMP=$(date +%Y%m%dT%H%M%S)

# Run note get synchronously with a 3s timeout. PreToolUse hooks
# block the agent until they exit; a hung binary (corrupt index.db,
# deadlocked SQLite, disk I/O stall) would wedge the session with no
# visible error. Three seconds is generous for a SQLite point-lookup
# but tight enough to fail fast on real hangs. macOS doesn't ship
# `timeout` by default — fall back to `gtimeout` (coreutils via brew),
# then to a no-timeout call as a last resort. Output discarded (the
# agent gets the body from Read itself); only exit status + first
# line matter for resolution detection.
TIMEOUT_CMD=""
if command -v timeout >/dev/null 2>&1; then
  TIMEOUT_CMD="timeout 3"
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_CMD="gtimeout 3"
fi
NOTE_OUTPUT=$(VAULTMIND_CALLER=vaultmind-preread-track $TIMEOUT_CMD "$VAULTMIND" note get "$REL_PATH" --vault "$VAULT_ROOT" 2>/dev/null)
NOTE_STATUS=$?

NOTE_RESOLVED=1
if [ "$NOTE_STATUS" != "0" ] || [ -z "$NOTE_OUTPUT" ]; then
  NOTE_RESOLVED=0
elif echo "$NOTE_OUTPUT" | head -1 | grep -q "^No note found"; then
  NOTE_RESOLVED=0
fi

if [ "$NOTE_RESOLVED" = "0" ]; then
  printf '{"timestamp":"%s","file_path":"%s","note_status":%d,"injected":false,"reason":"note_get_failed"}\n' \
    "$TIMESTAMP" "$FILE_PATH" "$NOTE_STATUS" \
    > "$LOG_DIR/${TIMESTAMP}-skip.json" 2>/dev/null
  exit 0
fi

# Inject a visible header via Claude Code's PreToolUse JSON contract.
# The agent sees additionalContext as model-visible context attached to
# this tool call — same channel as UserPromptSubmit injections, just
# scoped to the tool boundary.
HEADER="[vault-track-read] Read on vault note \"$REL_PATH\" — access recorded via \`vaultmind note get\`. Canonical retrieval for next time: vaultmind note get $REL_PATH --vault $VAULT_ROOT (or by id: vaultmind note get <id> --vault $VAULT_ROOT)."

python3 -c "
import json, sys
print(json.dumps({
    'hookSpecificOutput': {
        'hookEventName': 'PreToolUse',
        'additionalContext': sys.argv[1],
    }
}))
" "$HEADER"

printf '{"timestamp":"%s","file_path":"%s","injected":true,"header_chars":%d}\n' \
  "$TIMESTAMP" "$FILE_PATH" "${#HEADER}" \
  > "$LOG_DIR/${TIMESTAMP}-inject.json" 2>/dev/null

exit 0
