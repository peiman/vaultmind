#!/bin/bash
# PostToolUse hook — fire RecordNoteAccess when the agent uses the Read
# tool on a vault note, plugging the bypass that was draining the
# reinforcement signal.
#
# Why this exists (cross-session diagnosis, 2026-05-02): the
# pointers-only fix (28acebd) closed the SessionStart-preload bypass
# — agent reads vault content only by querying. But a second bypass
# stayed open: agent queries with `vaultmind ask`, gets a pointer
# list, then uses Claude Code's Read tool on the file path to fetch
# the body. Read is registered at the same tier as Bash, semantically
# above `vaultmind note get` (a Bash subcommand), so it wins on
# ergonomics. The result: bodies are read, but
# index.RecordNoteAccess never fires because Read doesn't go through
# the vaultmind binary. Reinforcement signal silently degrades.
#
# This hook intercepts Read PostToolUse and, when the path falls
# under a known vault directory, calls `vaultmind note get <path>`
# with output discarded. The resolution-by-path branch of note get
# triggers RecordNoteAccess; the printed body is dropped (the agent
# already has it from Read's return value). Net effect: every
# Read-on-vault becomes a tracked event without the agent learning a
# new habit.
#
# Caller labeling: events fire as "agent" because `note get` calls
# RecordNoteAccessAs with an explicit CallerAgent that wins over the
# env var (resolveCaller precedence: env-with-"hook" > explicit > env >
# default). The original design called for "agent-read" to let
# analytics break out Read-routed traffic from ask/note-get traffic;
# punted because it requires either (a) a --caller flag on note get,
# (b) a new dedicated `vaultmind index access` subcommand, or (c)
# changing resolveCaller's precedence. None of those is hard, none
# is in scope for the bypass-fix slice. The reinforcement signal is
# correct as of this commit; granular caller breakout is a follow-up.
#
# Output strategy: silent. Hooks shouldn't add noise — particularly
# PostToolUse hooks that fire on every Read. Failures are silently
# discarded; the user never sees anything from this hook in normal
# operation.

set -uo pipefail

# Read tool envelope from stdin (Claude Code JSON shape).
HOOK_INPUT=$(cat)

# Extract tool_name and tool_input.file_path. python3 over jq for
# parity with vault-recall.sh and to avoid an extra dep on systems
# where jq isn't installed.
TOOL_NAME=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_name',''))" 2>/dev/null || echo "")
FILE_PATH=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")

# Only fire on Read with a non-empty file path. Other tools (Bash,
# Grep, Edit, Write) don't go through this hook.
if [ "$TOOL_NAME" != "Read" ] || [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Path filter: only vault notes (markdown, under a vaultmind-* dir).
# Cheap regex — no need to walk filesystem looking for .vaultmind
# config dirs. The "vaultmind-" prefix is the convention this project
# uses for both shipped vaults and any user-init'd vault following
# the README.
case "$FILE_PATH" in
  */vaultmind-*/*.md) ;;
  *) exit 0 ;;
esac

# Resolve vault root: walk up from file path looking for a
# .vaultmind/ directory. This is robust to vaults at non-standard
# paths and to subdirectories (concepts/, arcs/, references/, etc.).
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

VAULTMIND="/tmp/vaultmind"
if [ ! -x "$VAULTMIND" ]; then
  # Substrate not ready — silently no-op. The SessionStart hook
  # surfaces build/wiring problems; this hook stays out of that
  # conversation.
  exit 0
fi

# Compute path relative to vault root for note get's resolver.
REL_PATH="${FILE_PATH#"$VAULT_ROOT"/}"

# Fire the access. Output discarded, errors discarded. Background
# (&) so the agent's Read return doesn't wait on this bookkeeping —
# the agent already has the body; access tracking is best-effort.
"$VAULTMIND" note get "$REL_PATH" --vault "$VAULT_ROOT" >/dev/null 2>&1 &

# Don't wait for the background access to complete — the agent
# already has the body from Read's return; this is just bookkeeping.
exit 0
