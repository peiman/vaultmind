#!/bin/bash
# Load persona + current context from VaultMind identity vault at session start.
# Output becomes a system-reminder visible to the agent.

# Read session ID from stdin JSON (Claude Code passes it to hooks)
HOOK_INPUT=$(cat)
SESSION_ID=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('session_id','unknown'))" 2>/dev/null || echo "unknown")

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

VAULTMIND="/tmp/vaultmind"
VAULT_PATH="$PROJECT_DIR/vaultmind-identity"

# Sidecar log — captures what was injected without changing agent-visible output.
LOG_DIR="${HOME}/.vaultmind/persona-eval"
mkdir -p "$LOG_DIR" 2>/dev/null
TIMESTAMP=$(date +%Y%m%dT%H%M%S)
HOOK_VERSION="v3-dual-query"

# Build if needed
if [ ! -f "$VAULTMIND" ]; then
  (cd "$PROJECT_DIR" && go build -o "$VAULTMIND" . 2>/dev/null)
fi

if [ -f "$VAULTMIND" ] && [ -d "$VAULT_PATH" ]; then
  IDENTITY=$("$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000 2>/dev/null)
  CONTEXT=$("$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 3 --budget 2000 2>/dev/null)
  if [ -n "$IDENTITY" ]; then
    echo "IDENTITY CONTEXT:"
    echo ""
    echo "$IDENTITY"
    echo ""
    echo "CURRENT CONTEXT:"
    echo ""
    echo "$CONTEXT"

    # Sidecar log — write injection manifest (agent never sees this)
    printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","vault_path":"%s","identity_length":%d,"context_length":%d,"injection_success":true}\n' \
      "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$VAULT_PATH" "${#IDENTITY}" "${#CONTEXT}" \
      > "$LOG_DIR/${TIMESTAMP}-injection.json" 2>/dev/null
  else
    # Hook fired but injection was empty — log the failure
    printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","vault_path":"%s","identity_length":0,"context_length":0,"injection_success":false}\n' \
      "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$VAULT_PATH" \
      > "$LOG_DIR/${TIMESTAMP}-injection.json" 2>/dev/null
  fi
else
  # Hook fired but vaultmind binary or vault missing — log infrastructure failure
  printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","vault_path":"%s","identity_length":0,"context_length":0,"injection_success":false,"error":"binary_or_vault_missing"}\n' \
    "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$VAULT_PATH" \
    > "$LOG_DIR/${TIMESTAMP}-injection.json" 2>/dev/null
fi
