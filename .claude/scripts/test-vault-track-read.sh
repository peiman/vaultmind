#!/bin/bash
# Smoke test for the vault-track-read PreToolUse hook (flavor B).
#
# Contract:
#   - Hook always exits 0 (non-blocking).
#   - On vault note Read whose path resolves to an indexed note:
#     emit JSON `hookSpecificOutput.additionalContext` with a header
#     naming the canonical `vaultmind note get` command. Fire access
#     tracking via note get.
#   - On non-vault path: silent.
#   - On non-Read tool: silent.
#   - On vault path that does NOT resolve to a real note (episodes,
#     unindexed files): silent — no misleading header.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HOOK="$SCRIPT_DIR/vault-track-read.sh"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
VAULT_PATH="$PROJECT_DIR/vaultmind-identity"

if [ ! -f "$HOOK" ]; then
  echo "FAIL: $HOOK does not exist" >&2
  exit 1
fi

if [ ! -x "/tmp/vaultmind" ]; then
  echo "SKIP: /tmp/vaultmind not built — run task build first" >&2
  exit 0
fi

if [ ! -d "$VAULT_PATH/.vaultmind" ]; then
  echo "SKIP: identity vault not initialized at $VAULT_PATH" >&2
  exit 0
fi

NOTE_FILE=$(find "$VAULT_PATH" -name "who-i-am.md" -type f | head -1)
if [ -z "$NOTE_FILE" ]; then
  NOTE_FILE=$(find "$VAULT_PATH" -name "*.md" -type f -not -path "*/episodes/*" | head -1)
fi
if [ -z "$NOTE_FILE" ]; then
  echo "SKIP: no markdown notes found" >&2
  exit 0
fi

TMP_STDOUT=$(mktemp)
TMP_STDERR=$(mktemp)
trap 'rm -f "$TMP_STDOUT" "$TMP_STDERR"' EXIT

# Case 1: Read on a vault note → exit 0, additionalContext on stdout.
INPUT=$(printf '{"tool_name":"Read","tool_input":{"file_path":"%s"}}' "$NOTE_FILE")
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?
if [ "$rc" != "0" ]; then
  echo "FAIL case 1: vault Read should exit 0 (non-blocking), got $rc" >&2
  echo "--- stderr ---" >&2; cat "$TMP_STDERR" >&2
  exit 1
fi
if ! grep -q "additionalContext" "$TMP_STDOUT"; then
  echo "FAIL case 1: stdout missing additionalContext JSON" >&2
  echo "--- stdout ---" >&2; cat "$TMP_STDOUT" >&2
  exit 1
fi
if ! grep -q "vaultmind note get" "$TMP_STDOUT"; then
  echo "FAIL case 1: additionalContext missing canonical-command hint" >&2
  cat "$TMP_STDOUT" >&2
  exit 1
fi
# Validate it parses as JSON.
python3 -c "import json,sys; json.load(open('$TMP_STDOUT'))" || {
  echo "FAIL case 1: stdout is not valid JSON" >&2
  cat "$TMP_STDOUT" >&2
  exit 1
}

# Case 2: Read on a non-vault path → silent, exit 0.
INPUT='{"tool_name":"Read","tool_input":{"file_path":"/tmp/some-random-file.txt"}}'
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?
if [ "$rc" != "0" ]; then
  echo "FAIL case 2: non-vault Read should exit 0, got $rc" >&2
  exit 1
fi
if [ -s "$TMP_STDOUT" ] || [ -s "$TMP_STDERR" ]; then
  echo "FAIL case 2: non-vault Read should be silent" >&2
  echo "--- stdout ---" >&2; cat "$TMP_STDOUT" >&2
  echo "--- stderr ---" >&2; cat "$TMP_STDERR" >&2
  exit 1
fi

# Case 3: Non-Read tool → silent, exit 0.
INPUT='{"tool_name":"Bash","tool_input":{"command":"ls"}}'
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?
if [ "$rc" != "0" ]; then
  echo "FAIL case 3: non-Read tool should exit 0, got $rc" >&2
  exit 1
fi
if [ -s "$TMP_STDOUT" ]; then
  echo "FAIL case 3: non-Read tool should be silent on stdout" >&2
  cat "$TMP_STDOUT" >&2
  exit 1
fi

# Case 4: Vault path that doesn't resolve → silent, exit 0 (no misleading
# header — note get returned "No note found").
INPUT=$(printf '{"tool_name":"Read","tool_input":{"file_path":"%s/nonexistent-xyz.md"}}' "$VAULT_PATH")
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?
if [ "$rc" != "0" ]; then
  echo "FAIL case 4: unresolved vault path should exit 0, got $rc" >&2
  exit 1
fi
if [ -s "$TMP_STDOUT" ]; then
  echo "FAIL case 4: unresolved vault path should not inject header" >&2
  cat "$TMP_STDOUT" >&2
  exit 1
fi

echo "PASS: vault-track-read hook smoke test (4 cases)"
