#!/bin/bash
# vault-code-map.sh — PreToolUse (Read|Edit|Write|MultiEdit).
#
# When the agent opens a code file, the notes whose `paths:` frontmatter covers
# that file arrive as a short map: title, id, one line. The knowledge about a
# file is found by the path the agent is already reading, at the moment it
# applies, with no query to guess. The agent opens what it needs with
# `vaultmind note get`.
#
# Once per file per session. Silent when nothing covers the file, when the file
# is itself a vault note, when no vault or binary is found, and when the binary
# predates `tree --for`. It adds context and never makes a permission decision:
# "allow" would let the tool skip the prompt the user's settings would show.
set -uo pipefail

HOOK_INPUT=$(cat)

# field <dotted.key> prints a string field of the hook input, or nothing.
field() {
  printf '%s' "$HOOK_INPUT" | python3 -c "
import json, sys
try:
    v = json.load(sys.stdin)
except Exception:
    sys.exit(0)
for k in sys.argv[1].split('.'):
    v = v.get(k) if isinstance(v, dict) else None
print(v if isinstance(v, str) else '')
" "$1" 2>/dev/null
}

FILE_PATH=$(field tool_input.file_path)
SESSION_ID=$(field session_id)
[ -n "$FILE_PATH" ] || exit 0
command -v vaultmind >/dev/null 2>&1 || exit 0

PROJECT_DIR="${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$(pwd)}}"
absolute() {
  case "$1" in
    /*) printf '%s' "${1%/}" ;;
    *) printf '%s' "$PROJECT_DIR/${1#./}" | sed 's:/*$::' ;;
  esac
}

VAULT="${VAULTMIND_VAULT:-}"
if [ -z "$VAULT" ]; then
  for cand in vaultmind-vault vaultmind-identity; do
    if [ -d "$PROJECT_DIR/$cand" ]; then VAULT="$PROJECT_DIR/$cand"; break; fi
  done
fi
[ -n "$VAULT" ] || exit 0
VAULT=$(absolute "$VAULT")
FILE_PATH=$(absolute "$FILE_PATH")

# A vault note is not code: the map is about the code the vault describes.
VAULT_LIST="$VAULT"
[ -n "${VAULTMIND_VAULTS:-}" ] && VAULT_LIST="$VAULT_LIST
$(printf '%s' "$VAULTMIND_VAULTS" | tr ',' '\n')"
while IFS= read -r v; do
  [ -n "$v" ] || continue
  case "$FILE_PATH" in "$(absolute "$v")"/*) exit 0 ;; esac
done <<< "$VAULT_LIST"

# Once per file per session.
LEDGER=""
if [ -n "$SESSION_ID" ]; then
  STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/vaultmind/code-map"
  mkdir -p "$STATE_DIR" 2>/dev/null
  LEDGER="$STATE_DIR/$(printf '%s' "$SESSION_ID" | tr -c 'A-Za-z0-9-' '_' | cut -c1-64)"
  if grep -Fxq -- "$FILE_PATH" "$LEDGER" 2>/dev/null; then exit 0; fi
fi

if [ -n "${VAULTMIND_VAULTS:-}" ]; then
  MAP_JSON=$(vaultmind tree --vaults "$VAULTMIND_VAULTS" --for "$FILE_PATH" --json 2>/dev/null) || exit 0
else
  MAP_JSON=$(vaultmind tree --vault "$VAULT" --for "$FILE_PATH" --json 2>/dev/null) || exit 0
fi

# Render the covering notes as the hook output; nothing when none cover it.
OUT=$(printf '%s' "$MAP_JSON" | python3 -c "$(cat <<'PY'
import json, os, sys
try:
    result = (json.load(sys.stdin) or {}).get("result") or {}
except Exception:
    sys.exit(0)

def notes(d):
    for n in d.get("notes") or []:
        yield n
    for sub in d.get("dirs") or []:
        yield from notes(sub)

lines, vaults = [], []
for v in result.get("vaults") or []:
    found = list(notes(v.get("root") or {}))
    if found:
        vaults.append(v.get("vault", ""))
    for n in found:
        line = "  %s (%s)" % (n.get("title", ""), n.get("id", ""))
        if n.get("line"):
            line += " \u2014 " + n["line"]
        if len(result.get("vaults") or []) > 1:
            line += "  [" + os.path.basename(v.get("vault", "").rstrip("/")) + "]"
        lines.append(line)
if not lines:
    sys.exit(0)
vault_hint = vaults[0] if len(vaults) == 1 else "<vault>"
text = ("VAULT \u2014 notes about " + (result.get("for") or "this file")
        + ", the file you are about to open. They were written for this code; "
        + "open one with: vaultmind note get <id> --vault " + vault_hint + "\n"
        + "\n".join(lines))
print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "additionalContext": text}}))
PY
)" 2>/dev/null)

[ -n "$OUT" ] || exit 0
printf '%s\n' "$OUT"
[ -n "$LEDGER" ] && printf '%s\n' "$FILE_PATH" >> "$LEDGER" 2>/dev/null
exit 0
