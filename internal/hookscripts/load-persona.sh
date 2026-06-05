#!/bin/bash
# Load persona + current context from VaultMind identity vault at session start.
# Output becomes a system-reminder visible to the agent.

# Read session ID from stdin JSON (Claude Code passes it to hooks)
HOOK_INPUT=$(cat)
SESSION_ID=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('session_id','unknown'))" 2>/dev/null || echo "unknown")

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

VAULTMIND_SRC="$PROJECT_DIR"
# Persona vault path — override per project for consumers whose
# identity vault has a different name. Default matches the vaultmind
# project convention. The companion project uses `companion-vault`; another
# project might use `vaultmind-knowledge`. Set inline in settings.json:
# `LOAD_PERSONA_VAULT="$CLAUDE_PROJECT_DIR/companion-vault" bash <script>`.
# Companion project 2026-05-07 HIGH-2 — silent empty-persona for non-default
# vault names was the dogfood-found regression.
#
# Precedence: LOAD_PERSONA_VAULT (persona-specific) wins, then the generic
# VAULTMIND_VAULT (set by `vaultmind hooks install --vault`, so one var drives
# every hook — issue #41.6), then the vaultmind-identity convention.
VAULT_PATH="${LOAD_PERSONA_VAULT:-${VAULTMIND_VAULT:-$PROJECT_DIR/vaultmind-identity}}"

# Sidecar log — captures what was injected without changing agent-visible output.
LOG_DIR="${HOME}/.vaultmind/persona-eval"
mkdir -p "$LOG_DIR" 2>/dev/null
TIMESTAMP=$(date +%Y%m%dT%H%M%S)
HOOK_VERSION="v5-self-state"

# Resolve the vaultmind binary:
#
# - Dev loop (vaultmind source dir present): build /tmp/vaultmind from
#   the local Go source when it's missing or stale. Keeps the dogfood
#   loop self-updating — any commit propagates to the next session
#   without a manual rm. /tmp/vaultmind is INTENTIONAL HERE; this is
#   the only place /tmp gets used, because it IS dev work.
#
# - Otherwise: use PATH-installed vaultmind (`task install` or
#   `go install`). Fail loudly if not on PATH so the agent doesn't
#   load silently-empty persona.
if [ -d "$VAULTMIND_SRC/internal" ] && [ -d "$VAULTMIND_SRC/cmd" ]; then
  # Dev loop.
  VAULTMIND="/tmp/vaultmind"
  needs_build=0
  if [ ! -f "$VAULTMIND" ]; then
    needs_build=1
  elif [ -n "$(find "$VAULTMIND_SRC" -name '*.go' -newer "$VAULTMIND" -print -quit 2>/dev/null)" ]; then
    needs_build=1
  fi
  if [ "$needs_build" = "1" ]; then
    # Delegate to the shared build script (SSOT for "rebuild vaultmind correctly").
    # Picks up -tags ORT when libtokenizers.a is present; falls back loudly
    # otherwise. See vaultmind#29.
    BUILD_OUTPUT=$(cd "$VAULTMIND_SRC" && bash .claude/scripts/build-vaultmind.sh "$VAULTMIND" 2>&1)
    BUILD_STATUS=$?
    if [ "$BUILD_STATUS" != "0" ]; then
      echo "VaultMind build failed — persona not loaded" >&2
      echo "$BUILD_OUTPUT" >&2
    fi
  fi
else
  # Not in dev loop — resolve PATH-installed binary.
  if command -v vaultmind >/dev/null 2>&1; then
    VAULTMIND=$(command -v vaultmind)
  else
    echo "VaultMind binary not on PATH. Install with 'task install' from the vaultmind repo." >&2
    VAULTMIND=""  # query block below skips silently when binary is missing
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
  #    already-someone (the "Hey <name>" greeting pattern). Stripping
  #    this to pointers would defeat that.
  #  - "what matters most right now" loads the CURRENT-STATE pointers
  #    only (--pointers-only). The agent gets titles + ids; the body of
  #    current-context is NOT preloaded. To learn what's actually current,
  #    the agent must explicitly query — which makes every body-read a
  #    real activation event instead of something the preload silently
  #    satisfied. This is the principle-9 fix for the dogfood-preload
  #    trap documented in the plasticity-gap arc and the
  #    2026-04-25 design signal under step 3 of the plasticity roadmap.
  ASK_ERR=$(mktemp -t vaultmind-persona-err.XXXXXX)
  IDENTITY=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000 2>"$ASK_ERR")
  IDENTITY_STATUS=$?
  CONTEXT=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 5 --budget 2000 --pointers-only 2>>"$ASK_ERR")

  # Self-state injection — surface the agent's own activation state
  # (recent / hot / stale notes) without requiring an explicit query.
  # Same template as the per-turn UserPromptSubmit pointers: ambient,
  # zero cognitive cost. Two vaults because the agent operates across
  # both: identity carries arcs/principles/references, research vault
  # carries the broader knowledge graph where most reinforcement signal
  # accumulates. Best-effort — if either fails, the persona above still
  # loads. See feat(self) commit and feedback_use_vaultmind_ask.
  # Optional research-vault second-query path. Default matches
  # vaultmind project convention; consumers with no separate research
  # vault either leave default (the `[ -d ]` guard below skips
  # silently if the dir doesn't exist) or set
  # LOAD_PERSONA_RESEARCH_VAULT to their second vault.
  #
  # NOTE: the research/second vault runs ONLY `vaultmind self` (the
  # memory/activation-state surface — hot/recent note titles), NOT a
  # content `ask`. It surfaces what's been reinforced in that vault,
  # not note bodies. So this block is cheap and ambient even on a large
  # research vault; it never preloads bodies the agent didn't query.
  RESEARCH_VAULT="${LOAD_PERSONA_RESEARCH_VAULT:-$PROJECT_DIR/vaultmind-vault}"
  SELF_IDENTITY=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" self --vault "$VAULT_PATH" --limit 5 2>>"$ASK_ERR" || true)
  SELF_RESEARCH=""
  if [ -d "$RESEARCH_VAULT" ]; then
    SELF_RESEARCH=$(VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" self --vault "$RESEARCH_VAULT" --limit 5 2>>"$ASK_ERR" || true)
  fi

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
    if [ -n "$SELF_IDENTITY" ]; then
      echo ""
      echo "MEMORY STATE — IDENTITY VAULT:"
      echo ""
      echo "$SELF_IDENTITY"
    fi
    if [ -n "$SELF_RESEARCH" ]; then
      echo ""
      echo "MEMORY STATE — RESEARCH VAULT:"
      echo ""
      echo "$SELF_RESEARCH"
    fi

    # Sidecar log — write injection manifest (agent never sees this)
    printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","vault_path":"%s","identity_length":%d,"context_length":%d,"self_identity_length":%d,"self_research_length":%d,"injection_success":true}\n' \
      "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$VAULT_PATH" "${#IDENTITY}" "${#CONTEXT}" "${#SELF_IDENTITY}" "${#SELF_RESEARCH}" \
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
