#!/bin/bash
# Load persona + current context from VaultMind identity vault at session start.
# Output becomes a system-reminder visible to the agent.

# Read session ID from stdin JSON (Claude Code passes it to hooks)
HOOK_INPUT=$(cat)
# The harness's conversation id, forwarded on every vaultmind call so this
# hook's events share a session with the agent's own (see vault-recall.sh).
HOOK_SESSION_ID=$(printf '%s' "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('session_id','') or '')" 2>/dev/null || echo "")
SESSION_ID=$(echo "$HOOK_INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('session_id','unknown'))" 2>/dev/null || echo "unknown")

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}}"

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
# One record per session start. The second alone is not unique: sessions in
# every project share this directory, and two starting in the same second
# overwrote each other. The session id (made filename-safe, and capped so the
# name stays under the 255-byte limit) separates them.
#
# Which part of the identity this run delivers (#357). load-arcs.sh runs this
# script with PART=arcs. This run leaves the arcs to it only when load-arcs.sh
# sits beside it AND a hook configuration an agent reads wires it: a script
# present but unwired would otherwise make the arcs vanish. When unsure it
# delivers everything, so the worst case is the arcs twice, never not at all.
arcs_hook_wired() {
  [ -f "$SCRIPT_DIR/load-arcs.sh" ] || return 1
  grep -qs 'load-arcs\.sh' \
    "$PROJECT_DIR/.claude/settings.json" "$PROJECT_DIR/.claude/settings.local.json" \
    "${CLAUDE_CONFIG_DIR:-$HOME/.claude}/settings.json" \
    "$PROJECT_DIR/.codex/hooks.json" "$HOME/.codex/hooks.json" \
    "$PROJECT_DIR/.cursor/hooks.json" "$HOME/.cursor/hooks.json"
}
PERSONA_PART="${VAULTMIND_PERSONA_PART:-}"
if [ -z "$PERSONA_PART" ]; then
  if arcs_hook_wired; then PERSONA_PART=identity; else PERSONA_PART=all; fi
fi
# Both hooks fire in the same second: the part keeps their records apart.
PART_TAG=""
[ "$PERSONA_PART" != arcs ] || PART_TAG="-arcs"
SIDECAR_FILE="$LOG_DIR/${TIMESTAMP}-$(printf '%s' "$SESSION_ID" | tr -c 'A-Za-z0-9-' '_' | cut -c1-64)${PART_TAG}-injection.json"
HOOK_VERSION="v8-arcs-hook"

# THE DELIVERY CAP (#355), per hook. Claude Code replaces a SessionStart hook's
# output with a ~2 KB preview once it passes about 10,000 characters, and the
# agent then starts from a preview of itself while this hook logs success.
OUTPUT_CAP="${VAULTMIND_HOOK_OUTPUT_CAP:-9000}"
# Bytes, whatever the locale: the cut was measured in bytes, and a byte count
# never undercounts characters. Callers compare with -ge, because the final
# newline printed after a block is one more byte.
bytes_of() { printf '%s' "$1" | wc -c | tr -d ' '; }

# Persona mode — how the identity reaches the agent:
#   served    (default) the identity, the arc layer and the current context are
#             queried here and handed over as text.
#   explore   nothing is handed over. The agent is told where its vaults are and
#             asked to go and look before it answers: remembering by looking,
#             not by being told.
#   alternate each session gets one of the two, split by session id, so the
#             arms can be compared on real sessions. The split is a function of
#             the id, so a compaction keeps the session in the arm it started in.
# The arm is recorded in the sidecar log (persona_mode) for measurement.
PERSONA_MODE="${VAULTMIND_PERSONA_MODE:-served}"
case "$PERSONA_MODE" in
  served|explore) ;;
  alternate)
    if [ -n "$HOOK_SESSION_ID" ] && [ $(( $(printf '%s' "$HOOK_SESSION_ID" | cksum | cut -d' ' -f1) % 2 )) = 1 ]; then
      PERSONA_MODE=explore
    else
      PERSONA_MODE=served
    fi
    ;;
  *)
    echo "VAULTMIND_PERSONA_MODE=$PERSONA_MODE is not served, explore or alternate — serving the identity" >&2
    PERSONA_MODE=served
    ;;
esac

# The explore arm hands over no identity, arcs included; its pointer to
# `arc recite` is in the persona hook's explore message.
if [ "$PERSONA_MODE" = explore ] && [ "$PERSONA_PART" = arcs ]; then
  printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","persona_mode":"%s","persona_part":"%s","vault_path":"%s","injection_success":true}\n' \
    "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$PERSONA_MODE" "$PERSONA_PART" "$VAULT_PATH" \
    > "$SIDECAR_FILE" 2>/dev/null
  exit 0
fi

if [ "$PERSONA_MODE" = explore ] && [ -d "$VAULT_PATH" ]; then
  # Places to look: the identity vault first, then every vault the project
  # names in VAULTMIND_VAULTS (the same list the reach and recall hooks use),
  # else the research vault when it exists.
  EXPLORE_VAULTS="$VAULT_PATH"
  if [ -n "${VAULTMIND_VAULTS:-}" ]; then
    EXTRA_VAULTS=$(printf '%s' "$VAULTMIND_VAULTS" | tr ',' '\n')
  else
    EXTRA_VAULTS="${LOAD_PERSONA_RESEARCH_VAULT:-$PROJECT_DIR/vaultmind-vault}"
  fi
  while IFS= read -r v; do
    v="${v%/}"
    [ -n "$v" ] && [ -d "$v" ] && [ "$v" != "${VAULT_PATH%/}" ] && EXPLORE_VAULTS="$EXPLORE_VAULTS
$v"
  done <<< "$EXTRA_VAULTS"

  echo "IDENTITY — EXPLORE:"
  echo ""
  echo "Who you are is not in this message. It is in your vault: your arcs (the"
  echo "moments that changed how you work), your principles, and what matters right"
  echo "now. You remember it by looking, not by being told. Be curious: before your"
  echo "first answer, go and find yourself, and follow what you find to whatever"
  echo "bears on what you are asked. Reflect on it, then answer."
  echo ""
  echo "Your vaults (identity first):"
  printf '%s\n' "$EXPLORE_VAULTS" | sed 's/^/  /'
  echo ""
  echo "Ways in:"
  echo "  vaultmind arc recite --vault $VAULT_PATH     every arc, by its principle"
  echo "  vaultmind ask \"<question>\" --vault <vault>"
  echo "  vaultmind search \"<words>\" --vault <vault>"
  echo "  vaultmind note get <id> --vault <vault>"

  printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","persona_mode":"%s","vault_path":"%s","injection_success":true}\n' \
    "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$PERSONA_MODE" "$VAULT_PATH" \
    > "$SIDECAR_FILE" 2>/dev/null
  exit 0
fi

# Resolve the vaultmind binary:
#
# - Dev loop (vaultmind's OWN source tree): build /tmp/vaultmind from
#   the local Go source when it's missing or stale. Keeps the dogfood
#   loop self-updating — any commit propagates to the next session
#   without a manual rm. /tmp/vaultmind is INTENTIONAL HERE; this is
#   the only place /tmp gets used, because it IS dev work. The tree is
#   recognised by its module path and its build script, not by having
#   internal/ and cmd/: that is the standard Go layout, and taking the
#   branch in someone else's Go project ran a build script they do not
#   have and loaded no persona (found by the 2026-10-02 audit).
#
# - Otherwise: use PATH-installed vaultmind (`task install` or
#   `go install`). Fail loudly if not on PATH so the agent doesn't
#   load silently-empty persona.
if grep -q '^module github.com/peiman/vaultmind$' "$VAULTMIND_SRC/go.mod" 2>/dev/null \
  && [ -f "$VAULTMIND_SRC/.claude/scripts/build-vaultmind.sh" ]; then
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

# THE ARC LAYER, in its own hook (#357). Every arc at a shorter excerpt beats
# fewer arcs at full length: measured on a 30-arc vault, --excerpt 120 is 10.5k
# (over the cap) and 70 is 8.6k (all thirty). So the excerpt steps down until
# the whole layer fits; if none does, the budget shrinks to the cap and
# `arc recite` names what it left out. A layer it cannot deliver is said so.
if [ "$PERSONA_PART" = arcs ]; then
  ARCS=""
  ARC_EXCERPT=0
  ARC_ERROR=""
  if [ -f "$VAULTMIND" ] && [ -d "$VAULT_PATH" ]; then
    ARC_ERR_FILE=$(mktemp -t vaultmind-arcs-err.XXXXXX)
    for ARC_EXCERPT in "${VAULTMIND_ARC_EXCERPT:-120}" 90 70 50; do
      ARCS=$(VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" arc recite --vault "$VAULT_PATH" --budget "${VAULTMIND_ARC_BUDGET:-3000}" --excerpt "$ARC_EXCERPT" 2>"$ARC_ERR_FILE")
      [ -n "$ARCS" ] && [ "$(bytes_of "$ARCS")" -ge "$OUTPUT_CAP" ] || break
    done
    if [ -n "$ARCS" ] && [ "$(bytes_of "$ARCS")" -ge "$OUTPUT_CAP" ]; then
      ARCS=$(VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" arc recite --vault "$VAULT_PATH" --budget $(( OUTPUT_CAP / 6 )) --excerpt "$ARC_EXCERPT" 2>"$ARC_ERR_FILE")
    fi
    if [ -n "$ARCS" ] && [ "$(bytes_of "$ARCS")" -ge "$OUTPUT_CAP" ]; then
      ARCS=""
      ARC_ERROR="the arc layer is over the hook delivery limit"
    fi
    [ -n "$ARCS" ] || [ -n "$ARC_ERROR" ] || ARC_ERROR=$(grep -m1 -i 'error' "$ARC_ERR_FILE" 2>/dev/null || head -n1 "$ARC_ERR_FILE" 2>/dev/null)
    rm -f "$ARC_ERR_FILE"
  else
    ARC_ERROR="the vaultmind binary or the identity vault is missing"
  fi
  if [ -n "$ARCS" ]; then
    printf '%s\n' "$ARCS"
    printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","persona_mode":"%s","persona_part":"%s","vault_path":"%s","output_bytes":%d,"arc_excerpt":%d,"injection_success":true}\n' \
      "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$PERSONA_MODE" "$PERSONA_PART" "$VAULT_PATH" "$(( $(bytes_of "$ARCS") + 1 ))" "$ARC_EXCERPT" \
      > "$SIDECAR_FILE" 2>/dev/null
  else
    ARC_ERROR=$(printf '%s' "${ARC_ERROR:-no output}" | tr -d '\\\000-\037' | tr '"' "'" | cut -c1-300)
    echo "ARC LAYER NOT LOADED this session — $ARC_ERROR."
    echo "Read your arcs before your first answer:"
    echo "  vaultmind arc recite --vault \"$VAULT_PATH\""
    printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","persona_mode":"%s","persona_part":"%s","vault_path":"%s","injection_success":false,"error":"%s"}\n' \
      "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$PERSONA_MODE" "$PERSONA_PART" "$VAULT_PATH" "$ARC_ERROR" \
      > "$SIDECAR_FILE" 2>/dev/null
  fi
  exit 0
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
  # --excerpt caps each of the 8 items at a bounded, decision-bearing passage.
  # Without it the budget is spent first-come: the earliest items arrive whole
  # and the rest arrive with no text at all. Measured on a real 63-note identity
  # vault: 3 of 8 with bodies and 5 with nothing. Capped, all 8 arrive.
  #
  # 300 is where the cap stops binding, measured on that vault:
  #   --excerpt  90 -> 716/6000    (the cap is trimming real content)
  #   --excerpt 300 -> 863/6000
  #   --excerpt 600 -> 863/6000    (identical — extraction is now the limit)
  # Above 300 the constraint is the length of a Principle section, not the cap,
  # so a larger number buys nothing. Below it, sections get cut.
  #
  # The pack deliberately leaves most of the 6,000 unspent: 8 decision rules
  # beat 3 whole arcs plus 5 titles for reconstruction. That trade is worth
  # revisiting if the budget is ever wanted for depth instead of breadth.
  #
  # The "what matters most right now" query below deliberately KEEPS
  # --pointers-only — see the preload-trap reasoning above. That one is a design
  # choice about forcing an explicit read, not an oversight.
  IDENTITY=$(VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000 --excerpt 300 2>"$ASK_ERR")
  IDENTITY_STATUS=$?

  # THE ARC LAYER, ENUMERATED — not retrieved (issue #47).
  #
  # `ask "who am I"` ranks. Measured on this 27-arc vault it delivered THREE
  # arcs, the same three on every run, because selection was similarity to that
  # literal string — which is uncorrelated with whatever the session is about.
  # 89% of the arc layer was not occasionally missed, it was permanently dark.
  #
  # `arc recite` enumerates instead: every arc, sorted by id, excerpted to its
  # Principle section (the rule) rather than its opening lines (the story
  # setup). Measured cost for all 27: 1,902 tokens against a 3,000 budget —
  # whole bodies would be 29,231, which is why this is excerpted rather than
  # simply unbounded. Anything that does not fit is named, not dropped.
  #
  # Best-effort: a failure here leaves the identity block above intact.
  # It runs below, once the other sections' size is known: the arc layer gets
  # the room they leave under the delivery cap (#355).
  CONTEXT=$(VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 5 --budget 2000 --pointers-only 2>>"$ASK_ERR")

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
  SELF_IDENTITY=$(VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" self --vault "$VAULT_PATH" --limit 5 2>>"$ASK_ERR" || true)
  SELF_RESEARCH=""
  if [ -d "$RESEARCH_VAULT" ]; then
    SELF_RESEARCH=$(VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" self --vault "$RESEARCH_VAULT" --limit 5 2>>"$ASK_ERR" || true)
  fi

  # The output stays under OUTPUT_CAP (#355; measured on a 30-arc vault: 15-19k
  # characters served, cut at 16 of 45 compactions in one long session). When
  # load-arcs.sh carries the arc layer, this hook leaves it out. When it does
  # not (an older install), the arc layer (the decision rules, #47) gets the room
  # the identity and current context leave (~6 characters per budget token);
  # the memory-state lists are worth less, so they are the first whole sections
  # to go when the total overshoots, each named with the command that reads it.
  ARCS=""
  OMITTED=""
  ARC_BUDGET=0
  if [ -n "$IDENTITY" ] && [ "$PERSONA_PART" = all ]; then
    ARC_BUDGET=$(( (OUTPUT_CAP - ${#IDENTITY} - ${#CONTEXT} - 700) / 6 ))
    [ "$ARC_BUDGET" -le "${VAULTMIND_ARC_BUDGET:-3000}" ] || ARC_BUDGET="${VAULTMIND_ARC_BUDGET:-3000}"
    if [ "$ARC_BUDGET" -ge 100 ]; then
      ARCS=$(VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" VAULTMIND_CALLER=vaultmind-persona-hook "$VAULTMIND" arc recite --vault "$VAULT_PATH" --budget "$ARC_BUDGET" --excerpt "${VAULTMIND_ARC_EXCERPT:-120}" 2>>"$ASK_ERR")
    else
      OMITTED="the arcs"
    fi
  fi

  # The first error line, made safe for the one-line JSON log.
  LOAD_ERROR=$(grep -m1 -i 'error' "$ASK_ERR" 2>/dev/null || head -n1 "$ASK_ERR" 2>/dev/null)
  LOAD_ERROR=$(printf '%s' "$LOAD_ERROR" | tr -d '\\\000-\037' | tr '"' "'" | cut -c1-300)
  if [ "$IDENTITY_STATUS" != "0" ]; then
    echo "VaultMind ask failed (exit $IDENTITY_STATUS) — persona not loaded" >&2
    cat "$ASK_ERR" >&2
  fi
  rm -f "$ASK_ERR"

  render_persona() {
    echo "IDENTITY CONTEXT:"
    echo ""
    echo "$IDENTITY"
    echo ""
    if [ -n "$ARCS" ]; then
      echo "$ARCS"
      echo ""
    fi
    if [ "$PERSONA_PART" = identity ]; then
      echo "(Your arcs arrive in their own message, from load-arcs.sh.)"
      echo ""
    fi
    if [ -n "$OMITTED" ]; then
      echo "Left out to stay under the hook delivery limit: $OMITTED. Read them with:"
      echo "  vaultmind arc recite --vault \"$VAULT_PATH\""
      echo "  vaultmind self --vault <vault>"
      echo ""
    fi
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
  }

  if [ -n "$IDENTITY" ]; then
    OUT=$(render_persona)
    if [ "$(bytes_of "$OUT")" -ge "$OUTPUT_CAP" ] && [ -n "$SELF_RESEARCH" ]; then
      SELF_RESEARCH=""; OMITTED="${OMITTED:+$OMITTED, }the research vault's memory state"; OUT=$(render_persona)
    fi
    if [ "$(bytes_of "$OUT")" -ge "$OUTPUT_CAP" ] && [ -n "$SELF_IDENTITY" ]; then
      SELF_IDENTITY=""; OMITTED="${OMITTED:+$OMITTED, }the identity vault's memory state"; OUT=$(render_persona)
    fi
    if [ "$(bytes_of "$OUT")" -ge "$OUTPUT_CAP" ] && [ -n "$ARCS" ]; then
      ARCS=""; OMITTED="${OMITTED:+$OMITTED, }the arcs"; OUT=$(render_persona)
    fi
    if [ "$(bytes_of "$OUT")" -ge "$OUTPUT_CAP" ]; then
      # The identity block alone is over the cap: a preview of it would be
      # worse than an honest pointer.
      OMITTED="everything"
      OUT=$(printf '%s\n' "IDENTITY TOO LARGE TO DELIVER in one hook (over $OUTPUT_CAP characters)." \
        "Reconstruct it before your first answer:" \
        "  vaultmind arc recite --vault \"$VAULT_PATH\"" \
        "  vaultmind ask \"who am I\" --vault \"$VAULT_PATH\"")
    fi
    printf '%s\n' "$OUT"

    # Sidecar log — write injection manifest (agent never sees this)
    printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","persona_mode":"%s","persona_part":"%s","vault_path":"%s","identity_length":%d,"context_length":%d,"self_identity_length":%d,"self_research_length":%d,"output_bytes":%d,"arc_budget":%d,"capped":"%s","injection_success":true}\n' \
      "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$PERSONA_MODE" "$PERSONA_PART" "$VAULT_PATH" "${#IDENTITY}" "${#CONTEXT}" "${#SELF_IDENTITY}" "${#SELF_RESEARCH}" "$(( $(bytes_of "$OUT") + 1 ))" "$ARC_BUDGET" "$OMITTED" \
      > "$SIDECAR_FILE" 2>/dev/null
  else
    # The load failed (#356). Say so where the agent reads, not only on stderr:
    # an absent identity must never look like a quiet start.
    echo "IDENTITY NOT LOADED this session — vaultmind could not read the identity vault:"
    echo "  ${LOAD_ERROR:-no output (exit $IDENTITY_STATUS)}"
    echo "Reconstruct it before your first answer:"
    echo "  vaultmind arc recite --vault \"$VAULT_PATH\""
    echo "  vaultmind ask \"who am I\" --vault \"$VAULT_PATH\""
    printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","persona_mode":"%s","persona_part":"%s","vault_path":"%s","identity_length":0,"context_length":0,"injection_success":false,"error":"%s"}\n' \
      "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$PERSONA_MODE" "$PERSONA_PART" "$VAULT_PATH" "$LOAD_ERROR" \
      > "$SIDECAR_FILE" 2>/dev/null
  fi
else
  # Hook fired but vaultmind binary or vault missing — log infrastructure failure
  printf '{"timestamp":"%s","session_id":"%s","term_session_id":"%s","hook_version":"%s","persona_mode":"%s","vault_path":"%s","identity_length":0,"context_length":0,"injection_success":false,"error":"binary_or_vault_missing"}\n' \
    "$TIMESTAMP" "$SESSION_ID" "${TERM_SESSION_ID:-}" "$HOOK_VERSION" "$PERSONA_MODE" "$VAULT_PATH" \
    > "$SIDECAR_FILE" 2>/dev/null
fi
