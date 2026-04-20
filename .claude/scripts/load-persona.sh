#!/bin/bash
# Load persona + current context from VaultMind identity vault at session start.
# Output becomes a system-reminder visible to the agent.

# Read session ID from stdin JSON (Claude Code passes it to hooks)
HOOK_INPUT=$(cat)
SESSION_ID=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('session_id','unknown'))" 2>/dev/null || echo "unknown")

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

VAULTMIND="/tmp/vaultmind"
VAULTMIND_SRC="$PROJECT_DIR"
VAULT_PATH="$PROJECT_DIR/vaultmind-identity"

# Sidecar log — captures what was injected without changing agent-visible output.
LOG_DIR="${HOME}/.vaultmind/persona-eval"
mkdir -p "$LOG_DIR" 2>/dev/null
TIMESTAMP=$(date +%Y%m%dT%H%M%S)
HOOK_VERSION="v4-hardened"

# Rebuild when binary is absent OR any .go source is newer than the binary.
# Keeps the dogfood loop self-updating — any VaultMind commit propagates to
# the next session without a manual rm.
needs_build=0
if [ ! -f "$VAULTMIND" ]; then
  needs_build=1
elif [ -d "$VAULTMIND_SRC" ] && [ -n "$(find "$VAULTMIND_SRC" -name '*.go' -newer "$VAULTMIND" -print -quit 2>/dev/null)" ]; then
  needs_build=1
fi

if [ "$needs_build" = "1" ] && [ -d "$VAULTMIND_SRC" ]; then
  BUILD_OUTPUT=$(cd "$VAULTMIND_SRC" && go build -o "$VAULTMIND" . 2>&1)
  BUILD_STATUS=$?
  if [ "$BUILD_STATUS" != "0" ]; then
    # Surface build failures instead of silently loading no persona.
    echo "VaultMind build failed — persona not loaded" >&2
    echo "$BUILD_OUTPUT" >&2
  fi
fi

if [ -f "$VAULTMIND" ] && [ -d "$VAULT_PATH" ]; then
  # Capture stderr so runtime failures surface instead of producing empty
  # persona silently. VAULTMIND_CALLER tags the event so the experiment DB
  # can separate hook-triggered loads from deliberate queries.
  ASK_ERR=$(mktemp -t vaultmind-persona-err.XXXXXX)
  IDENTITY=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000 2>"$ASK_ERR")
  IDENTITY_STATUS=$?
  CONTEXT=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 3 --budget 2000 2>>"$ASK_ERR")
  if [ "$IDENTITY_STATUS" != "0" ]; then
    echo "VaultMind ask failed (exit $IDENTITY_STATUS) — persona not loaded" >&2
    cat "$ASK_ERR" >&2
  fi
  rm -f "$ASK_ERR"

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
