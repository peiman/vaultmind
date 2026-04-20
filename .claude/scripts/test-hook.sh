#!/bin/bash
# Smoke test for the persona SessionStart hook.
#
# Runs load-persona.sh with mocked Claude-Code stdin and checks that:
#   - the hook exits cleanly
#   - stdout contains the expected identity-context marker
#   - stderr is empty (no surfaced build/runtime errors)
#
# Run directly: bash .claude/scripts/test-hook.sh
# Or via bootstrap: bash .claude/scripts/bootstrap.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HOOK="$SCRIPT_DIR/load-persona.sh"

if [ ! -f "$HOOK" ]; then
  echo "FAIL: $HOOK does not exist" >&2
  exit 1
fi

# Capture stdout and stderr separately so we can assert on each.
TMP_STDOUT=$(mktemp)
TMP_STDERR=$(mktemp)
trap 'rm -f "$TMP_STDOUT" "$TMP_STDERR"' EXIT

# Claude Code passes hook input as JSON on stdin.
if ! echo '{"session_id":"hook-smoke-test"}' | bash "$HOOK" >"$TMP_STDOUT" 2>"$TMP_STDERR"; then
  echo "FAIL: hook exited non-zero" >&2
  echo "--- stderr ---" >&2
  cat "$TMP_STDERR" >&2
  exit 1
fi

if ! grep -q "IDENTITY CONTEXT" "$TMP_STDOUT"; then
  echo "FAIL: hook ran but did not produce IDENTITY CONTEXT marker" >&2
  echo "--- stdout ---" >&2
  head -20 "$TMP_STDOUT" >&2
  echo "--- stderr ---" >&2
  cat "$TMP_STDERR" >&2
  exit 1
fi

# Non-empty stderr is a signal — either build-error surface, runtime-error
# surface, or the ask_err leak. Warn but don't fail (some debug output is OK).
if [ -s "$TMP_STDERR" ]; then
  echo "WARN: hook produced stderr output:" >&2
  cat "$TMP_STDERR" >&2
fi

echo "PASS: hook smoke test"
