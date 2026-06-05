#!/bin/bash
# UserPromptSubmit hook — query the identity vault for pointers relevant to
# the user's prompt and inject them as context the model sees.
#
# This is the second slice of the plasticity roadmap, step 3 (activation-
# triggered recall). The first slice (commit 28acebd) made SessionStart
# preload pointers-only on the current-state slice. This slice extends the
# query-then-read loop to mid-session: every user message triggers a
# pointers query against the identity vault, and the agent sees the pointer
# menu before responding.
#
# Why this is the principle-9 fix at per-turn cadence: instead of relying
# on the agent to remember to query before answering, the SYSTEM queries
# automatically. The agent sees pointers (not bodies) and chooses whether
# to dig (explicit `vaultmind ask <id>`) or proceed without. Discipline →
# design, applied at every turn instead of just session start.
#
# Output strategy: low-noise. Skip silently when the prompt is too short
# to be worth querying, when the substrate isn't ready, or when the query
# returns nothing useful. The agent's signal-to-noise ratio matters; this
# hook should help, not clutter.

# Read prompt from stdin JSON
HOOK_INPUT=$(cat)
PROMPT=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('prompt',''))" 2>/dev/null || echo "")

# Single-word / command-style messages aren't worth a vault query. The
# threshold is rough — favors silence over noise. Real topical questions
# usually have at least a sentence-fragment shape.
if [ -z "$PROMPT" ] || [ "${#PROMPT}" -lt 12 ]; then
  exit 0
fi

# Use PATH-installed vaultmind. /tmp/vaultmind is the dev-loop binary
# (auto-rebuilt by load-persona.sh on Go-source change) and not a
# valid fallback for general use — users install via `task install`.
# Silently skip if not on PATH; load-persona.sh is the loud surface.
if ! command -v vaultmind >/dev/null 2>&1; then
  exit 0
fi
VAULTMIND=$(command -v vaultmind)
# Per-concern env routing: VAULTMIND_RECALL_VAULT points *per-turn recall*
# at its own vault, independent of persona-load and episode-write. It falls
# back to the overloaded VAULTMIND_VAULT (set by `vaultmind hooks install
# --vault`, and the simple single-var default), then to the vaultmind-identity
# convention. A dual-vault adopter can route recall, episodes, and persona
# independently; a single-var setup is unchanged (issue #41.6).
VAULT_PATH="${VAULTMIND_RECALL_VAULT:-${VAULTMIND_VAULT:-$CLAUDE_PROJECT_DIR/vaultmind-identity}}"

# Substrate not ready — silently no-op.
if [ ! -d "$VAULT_PATH" ]; then
  exit 0
fi

# Sidecar log directory — captures invocations without changing agent-visible
# output. Used to verify the hook is firing when expected and to study how
# often pointers actually surface useful targets.
LOG_DIR="${HOME}/.vaultmind/userprompt-hook"
mkdir -p "$LOG_DIR" 2>/dev/null
TIMESTAMP=$(date +%Y%m%dT%H%M%S)

# Pointers-only ask, low max-items to keep noise bounded. VAULTMIND_CALLER
# tags the event in the experiment DB so we can separate per-turn auto-recall
# events from explicit user queries.
#
# --quiet-on-no-match is the relevance floor: when the prompt is off-domain
# (top hit at/below the embedder's noise floor), ask prints nothing, so
# POINTERS is empty and the [ -z "$POINTERS" ] gate below injects silence
# instead of irrelevant pointers. It also skips the access fan-out, so
# off-domain prompts don't reinforce the notes they happened to surface.
ASK_ERR=$(mktemp -t vaultmind-userprompt-err.XXXXXX)
POINTERS=$(VAULTMIND_CALLER=vaultmind-userprompt-hook "$VAULTMIND" ask "$PROMPT" \
  --vault "$VAULT_PATH" \
  --max-items 3 \
  --budget 1500 \
  --quiet-on-no-match \
  --pointers-only 2>"$ASK_ERR")
ASK_STATUS=$?

if [ "$ASK_STATUS" != "0" ] || [ -z "$POINTERS" ]; then
  # Log the failure to the sidecar but don't surface it to the agent — a
  # broken vault recall shouldn't block the user's message.
  printf '{"timestamp":"%s","prompt_len":%d,"ask_status":%d,"injection":false,"error":%s}\n' \
    "$TIMESTAMP" "${#PROMPT}" "$ASK_STATUS" "$(cat "$ASK_ERR" | python3 -c "import json,sys; print(json.dumps(sys.stdin.read()))" 2>/dev/null || echo '""')" \
    > "$LOG_DIR/${TIMESTAMP}-skip.json" 2>/dev/null
  rm -f "$ASK_ERR"
  exit 0
fi
rm -f "$ASK_ERR"

# Inject the pointers with a clear header that names the next move. The
# agent should treat these as a menu — query for body if relevant, ignore
# if not. The header phrasing avoids commanding ("you must read this") —
# we want activation, not coercion.
echo "VAULT POINTERS related to your message (identity vault — run 'vaultmind ask <id> --vault $VAULT_PATH' to read body):"
echo ""
echo "$POINTERS"

# Log the successful injection
printf '{"timestamp":"%s","prompt_len":%d,"ask_status":0,"injection":true,"pointer_chars":%d}\n' \
  "$TIMESTAMP" "${#PROMPT}" "${#POINTERS}" \
  > "$LOG_DIR/${TIMESTAMP}-inject.json" 2>/dev/null
