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
  # Delegate to the shared build script (SSOT for "rebuild vaultmind correctly").
  # The script picks up -tags ORT when libtokenizers.a is present and falls
  # back loudly otherwise. See vaultmind#29.
  BUILD_OUTPUT=$(cd "$VAULTMIND_SRC" && bash .claude/scripts/build-vaultmind.sh "$VAULTMIND" 2>&1)
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
  #
  # The two queries serve different purposes:
  #  - "who am I" loads the IDENTITY anchor with full bodies. Priming.
  #    Cross-session continuity depends on the agent showing up as
  #    already-someone (the workhorse "Hey Peiman" pattern). Stripping
  #    this to pointers would defeat that.
  #  - "what matters most right now" loads the CURRENT-STATE pointers
  #    only (--pointers-only). The agent gets titles + ids; the body of
  #    current-context is NOT preloaded. To learn what's actually current,
  #    the agent must explicitly query — which makes every body-read a
  #    real activation event instead of something the preload silently
  #    satisfied. This is the principle-9 fix for the dogfood-preload
  #    trap documented in arc-plasticity-gap-from-inside and the
  #    2026-04-25 design signal under step 3 of plasticity-priority-order.
  ASK_ERR=$(mktemp -t vaultmind-persona-err.XXXXXX)
  IDENTITY=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000 2>"$ASK_ERR")
  IDENTITY_STATUS=$?
  CONTEXT=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 5 --budget 2000 --pointers-only 2>>"$ASK_ERR")
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
