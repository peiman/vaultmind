#!/bin/bash
# Smoke test for the vault-block-read PreToolUse hook.
#
# Cases covered:
#   1. Read on a vault note → hook blocks (exit 2), surfaces body on stderr
#   2. Read on a non-vault path → hook is a no-op (exit 0, silent)
#   3. Non-Read tool → hook is a no-op (exit 0, silent)
#   4. Vault path with note get failure → hook allows Read (exit 0)
#
# Run directly: bash .claude/scripts/test-vault-block-read.sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HOOK="$SCRIPT_DIR/vault-block-read.sh"
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

# Pick an existing note to test against. The "who am I" identity note
# is load-bearing and always present.
NOTE_FILE=$(find "$VAULT_PATH" -name "who-i-am.md" -type f | head -1)
if [ -z "$NOTE_FILE" ]; then
  # Fall back to any markdown file in the vault.
  NOTE_FILE=$(find "$VAULT_PATH" -name "*.md" -type f -not -path "*/episodes/*" | head -1)
fi
if [ -z "$NOTE_FILE" ]; then
  echo "SKIP: no markdown notes found in $VAULT_PATH" >&2
  exit 0
fi

TMP_STDOUT=$(mktemp)
TMP_STDERR=$(mktemp)
trap 'rm -f "$TMP_STDOUT" "$TMP_STDERR"' EXIT

# Case 1: Read on a vault note → block.
INPUT=$(printf '{"tool_name":"Read","tool_input":{"file_path":"%s"}}' "$NOTE_FILE")
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?

if [ "$rc" != "2" ]; then
  echo "FAIL case 1: expected exit 2 (block), got $rc" >&2
  echo "--- stdout ---" >&2; cat "$TMP_STDOUT" >&2
  echo "--- stderr ---" >&2; cat "$TMP_STDERR" >&2
  exit 1
fi

if ! grep -q "vault-block-read" "$TMP_STDERR"; then
  echo "FAIL case 1: stderr missing vault-block-read header" >&2
  cat "$TMP_STDERR" >&2
  exit 1
fi

if ! grep -q "vaultmind note get" "$TMP_STDERR"; then
  echo "FAIL case 1: stderr missing 'vaultmind note get' canonical-command hint" >&2
  exit 1
fi

# Case 2: Read on a non-vault path → no-op.
INPUT='{"tool_name":"Read","tool_input":{"file_path":"/tmp/some-random-file.txt"}}'
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?
if [ "$rc" != "0" ]; then
  echo "FAIL case 2: non-vault Read should exit 0, got $rc" >&2
  cat "$TMP_STDERR" >&2
  exit 1
fi
if [ -s "$TMP_STDOUT" ] || [ -s "$TMP_STDERR" ]; then
  echo "FAIL case 2: non-vault Read should be silent" >&2
  echo "--- stdout ---" >&2; cat "$TMP_STDOUT" >&2
  echo "--- stderr ---" >&2; cat "$TMP_STDERR" >&2
  exit 1
fi

# Case 3: Non-Read tool → no-op.
INPUT='{"tool_name":"Bash","tool_input":{"command":"ls"}}'
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?
if [ "$rc" != "0" ]; then
  echo "FAIL case 3: non-Read tool should exit 0, got $rc" >&2
  exit 1
fi

# Case 4: Vault path that doesn't resolve to a real note → allow Read.
INPUT=$(printf '{"tool_name":"Read","tool_input":{"file_path":"%s/nonexistent-note-xyz.md"}}' "$VAULT_PATH")
echo "$INPUT" | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR" && rc=$? || rc=$?
if [ "$rc" != "0" ]; then
  echo "FAIL case 4: vault path with note get failure should exit 0 (allow Read), got $rc" >&2
  cat "$TMP_STDERR" >&2
  exit 1
fi

echo "PASS: vault-block-read hook smoke test (4 cases)"
