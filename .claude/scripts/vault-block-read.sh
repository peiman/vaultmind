#!/bin/bash
# STATUS: PARKED — not wired in .claude/settings.json. The active flavor is
# vault-track-read.sh (non-blocking, header-injection). This file is preserved
# unwired as the documented escalation per arc-the-lighter-move-is-the-work;
# enable only if dogfood data shows the non-blocking variant is insufficient.
# DO NOT WIRE without re-reading reference-activation-rerank-decision and the
# arc above.
#
# PreToolUse hook on Read — when the agent tries to Read a vault note,
# block the Read and surface the body via `vaultmind note get` instead.
#
# Why this exists (cross-session diagnosis, 2026-05-02): the
# PostToolUse vault-track-read.sh hook (commit f219f0e) closed the
# bookkeeping bypass — Read on vault files now records access. But
# the *behavior* bypass stayed open: Read is a tool primitive, while
# `vaultmind note get` is a Bash sub-ceremony with a long path and
# --vault flag. Read wins on ergonomics, so the agent keeps reaching
# for it, and never internalizes the canonical retrieval idiom. This
# hook closes the behavior side: for vault notes, Read does not
# exist. The right command is the only path.
#
# Mechanism: detect vault paths, run `vaultmind note get` synchronously
# (which fires RecordNoteAccess and returns the formatted body), surface
# the body on stderr, exit 2 — Claude Code's PreToolUse block contract.
# The agent sees the body AND a header naming the right command for
# next time.
#
# Failure handling: if `note get` fails (binary missing, path doesn't
# resolve to a real indexed note — episode transcripts, malformed
# frontmatter, transient errors), allow Read to proceed. Better to let
# the agent continue than to wedge them on infrastructure quirks. The
# PostToolUse tracker remains as a safety net.
#
# Edit caveat: this breaks the Edit tool on vault notes because Edit's
# precondition requires Read to have been called successfully. If Edit
# friction becomes real, add an explicit unlock — env var, marker file,
# or a `vaultmind note edit` subcommand that opens an editor on the
# resolved path. Don't pre-design — wait for the friction to surface.

set -uo pipefail

HOOK_INPUT=$(cat)

TOOL_NAME=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_name',''))" 2>/dev/null || echo "")
FILE_PATH=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")

# Only fire on Read with a non-empty file path.
if [ "$TOOL_NAME" != "Read" ] || [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Path filter: only vault notes (markdown, under a vaultmind-* dir).
case "$FILE_PATH" in
  */vaultmind-*/*.md) ;;
  *) exit 0 ;;
esac

# Resolve vault root: walk up looking for .vaultmind/.
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
  # Substrate not ready — silently no-op so missing-binary doesn't
  # wedge the agent. The SessionStart hook surfaces build problems.
  exit 0
fi

REL_PATH="${FILE_PATH#"$VAULT_ROOT"/}"

# Sidecar log directory.
LOG_DIR="${HOME}/.vaultmind/preread-block"
mkdir -p "$LOG_DIR" 2>/dev/null
TIMESTAMP=$(date +%Y%m%dT%H%M%S)

# Run `note get`. Capture stdout (the body) and exit status.
NOTE_OUTPUT=$(VAULTMIND_CALLER=vaultmind-preread-block "$VAULTMIND" note get "$REL_PATH" --vault "$VAULT_ROOT" 2>/dev/null)
NOTE_STATUS=$?

# Allow Read if note get fails — episode files, malformed frontmatter,
# transient errors. PostToolUse tracker still fires as fallback.
#
# `vaultmind note get` exits 0 even for unresolved ids (it prints
# "No note found for ..." to stdout). Detect that pattern explicitly —
# without it, episodes and unindexed files would be blocked with a
# bogus body that doesn't match the file.
NOTE_RESOLVED=1
if [ "$NOTE_STATUS" != "0" ] || [ -z "$NOTE_OUTPUT" ]; then
  NOTE_RESOLVED=0
elif echo "$NOTE_OUTPUT" | head -1 | grep -q "^No note found"; then
  NOTE_RESOLVED=0
fi
if [ "$NOTE_RESOLVED" = "0" ]; then
  printf '{"timestamp":"%s","file_path":"%s","note_status":%d,"blocked":false,"reason":"note_get_failed"}\n' \
    "$TIMESTAMP" "$FILE_PATH" "$NOTE_STATUS" \
    > "$LOG_DIR/${TIMESTAMP}-skip.json" 2>/dev/null
  exit 0
fi

# Block the Read. stderr is shown to the agent as the block reason.
{
  echo "[vault-block-read] Read intercepted on vault note: $FILE_PATH"
  echo ""
  echo "Vault notes have a tracked retrieval path. The body has been"
  echo "fetched via \`vaultmind note get\` (access tracked in the"
  echo "experiment DB) and is shown below. For next time, prefer:"
  echo ""
  echo "  vaultmind note get $REL_PATH --vault $VAULT_ROOT"
  echo ""
  echo "or by id:"
  echo ""
  echo "  vaultmind note get <id>  --vault $VAULT_ROOT"
  echo ""
  echo "----- BEGIN NOTE BODY -----"
  echo "$NOTE_OUTPUT"
  echo "----- END NOTE BODY -----"
  echo ""
  echo "(If you genuinely need the raw file bytes — e.g. for Edit — this"
  echo "block currently has no escape hatch. See .claude/scripts/vault-block-read.sh"
  echo "for the design tradeoff.)"
} >&2

# Log the block.
printf '{"timestamp":"%s","file_path":"%s","blocked":true,"body_chars":%d}\n' \
  "$TIMESTAMP" "$FILE_PATH" "${#NOTE_OUTPUT}" \
  > "$LOG_DIR/${TIMESTAMP}-block.json" 2>/dev/null

# Exit 2 → Claude Code blocks the tool and shows stderr to the agent.
exit 2
