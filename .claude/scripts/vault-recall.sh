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

# Not every UserPromptSubmit payload is a human asking something. Background
# task completions, hook output and system reminders arrive through this SAME
# `prompt` field, and ranking notes against a task-id envelope produces pointers
# about nothing — retrieval working correctly on garbage input. These payloads
# are long, so the length threshold above waves them straight through.
#
# Measured on one real session before this guard existed: 93 injections, 48% of
# them ranked against a prompt over 250 chars that was a <task-notification>
# envelope — roughly 103k pointer characters of noise. Half a channel's output
# being noise is precisely how an agent learns to skip the channel, and then
# misses the half that mattered. That is a tooling defect, not a discipline
# problem: the fix belongs here, not in a resolution to read more carefully.
#
# Matched anywhere in the prompt rather than only at the start — a notification
# envelope is often preceded by a banner line. The ANGLE-BRACKETED form is what
# separates a payload from prose about one, so "why does the hook skip a
# system-reminder?" still queries; typing the brackets out does not. That is an
# accepted, rare cost in exchange for silence on the common case.
case "$PROMPT" in
  *"<task-notification>"*|*"<system-reminder>"*|*"<local-command-"*|*"hook success:"*)
    exit 0
    ;;
esac

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

# Bound the query. Claude Code kills a UserPromptSubmit hook at its own budget
# and DISCARDS the output, so an unbounded query on a loaded machine spends the
# whole budget and injects nothing — the turn pays and gets nothing back.
# Better to give up early and stay silent: pointers are a courtesy, and this
# hook is fail-open by design.
#
# macOS doesn't ship `timeout` — same fallback chain vault-track-read.sh uses:
# `timeout`, then `gtimeout` (coreutils via brew), then an unbounded call.
# VAULTMIND_HOOK_QUERY_TIMEOUT tunes it for a large vault or a slow machine.
TIMEOUT_CMD=""
if command -v timeout >/dev/null 2>&1; then
  TIMEOUT_CMD="timeout ${VAULTMIND_HOOK_QUERY_TIMEOUT:-15}"
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_CMD="gtimeout ${VAULTMIND_HOOK_QUERY_TIMEOUT:-15}"
fi

# Pointers-only ask, low max-items to keep noise bounded. VAULTMIND_CALLER
# tags the event in the experiment DB so we can separate per-turn auto-recall
# events from explicit user queries.
#
# --quiet-on-no-match is the relevance floor: when the prompt is off-domain
# (top hit at/below the embedder's noise floor), ask prints nothing, so
# POINTERS is empty and the [ -z "$POINTERS" ] gate below injects silence
# instead of irrelevant pointers. It also skips the access fan-out, so
# off-domain prompts don't reinforce the notes they happened to surface.
# A3 query-style rewrite (2026-08-28): connective retrieval phrases carry no
# ranking signal and measurably depress it — probed raw-vs-rewritten against a
# frozen set, "can you find our notes about spreading activation" ranked the
# right note 3rd while the stripped query ranks it 1st (Δz +2.59). META-ONLY
# by measurement: the variant that also stripped interrogative frames caused
# the probe's only rank regressions, so frames stay. Stripping that leaves
# fewer than 2 words falls back to the raw prompt, as does any python failure.
# Probe + decision record: docs/reviews/a3-query-rewrite/ (private repo).
QUERY=$(VM_RAW_PROMPT="$PROMPT" python3 - <<'PYEOF' 2>/dev/null
import os
import re

q = os.environ.get("VM_RAW_PROMPT", "").strip()
meta = re.compile(
    r"\b(?:information\s+about|records?\s+of|(?:our|my|the)\s+notes?\s+(?:about|on)"
    r"|notes?\s+(?:about|on)|anything\s+about|please|again|can\s+you\s+find"
    r"|find\s+(?:me|our|my)|search\s+for|look\s+up|show\s+me"
    r"|tell\s+me\s+(?:more\s+about|about)|give\s+me|remind\s+me\s+(?:about|of))\b",
    re.IGNORECASE,
)
s = meta.sub(" ", q).rstrip("?!. ")
s = re.sub(r"\s+", " ", s).strip()
print(s if len(s.split()) >= 2 else q)
PYEOF
)
[ -z "$QUERY" ] && QUERY="$PROMPT"

ASK_ERR=$(mktemp -t vaultmind-userprompt-err.XXXXXX)
POINTERS=$(VAULTMIND_CALLER=vaultmind-userprompt-hook $TIMEOUT_CMD "$VAULTMIND" ask "$QUERY" \
  --vault "$VAULT_PATH" \
  --max-items 3 \
  --budget 1500 \
  --quiet-on-no-match \
  --excerpt 80 2>"$ASK_ERR")
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

# Inject with a header that names what is actually here. The phrasing avoids
# commanding ("you must read this") — we want activation, not coercion.
#
# This used to say "run 'vaultmind ask <id>' to read body", which was true when
# the query passed --pointers-only and no body was ever included. With --excerpt
# the decision-bearing passage is inline, so the old line would be pointing at a
# fetch for content already on the screen. The fetch is still named, as the way
# to get the WHOLE note rather than the way to get anything at all.
echo "VAULT — from your own notes, relevant to what you just said:"
echo ""
echo "$POINTERS"
echo ""
echo "(the note's own text — a Principle section where it has one, else its opening lines. Full note: vaultmind note get <id> --vault $VAULT_PATH)"

# Log the successful injection
printf '{"timestamp":"%s","prompt_len":%d,"ask_status":0,"injection":true,"pointer_chars":%d}\n' \
  "$TIMESTAMP" "${#PROMPT}" "${#POINTERS}" \
  > "$LOG_DIR/${TIMESTAMP}-inject.json" 2>/dev/null
