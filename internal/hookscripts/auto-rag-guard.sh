#!/bin/bash
# auto-rag-guard.sh — PreToolUse hook for in-loop drift detection.
#
# Catches known auto-mode drift patterns, queries vaultmind for the
# canonical guidance, and surfaces the result via additionalContext so
# the next moment of the agent's reasoning has the relevant memory in
# scope. Logs every firing for vaultmind feedback aggregation.
#
# Canonical engine — distributed via `vaultmind hooks install` (absorbed
# from the companion project v0.3 stable, 2026-05-07). Consumers either use the
# defaults below or override with env vars:
#   AUTORAG_VAULT          Vault to query (default:
#                          $CLAUDE_PROJECT_DIR/vaultmind-identity, the
#                          VaultMind project convention; consumers
#                          whose vault dir has a different name MUST
#                          set this).
#   AUTORAG_ALLOWED_ROOTS  Colon-separated allowlist for cross-project
#                          Write/Edit (default:
#                          $CLAUDE_PROJECT_DIR:$HOME/.claude:/tmp).
#                          Limitation: paths must not contain literal
#                          colons (same shape as PATH/MANPATH).
#   VAULTMIND_BIN          Path to vaultmind (default: PATH-installed
#                          `vaultmind`; falls back to /tmp/vaultmind).
#   DRIFT_CATALOG          JSON array of consumer-supplied drift
#                          signatures, matching the schema in
#                          internal/hooks/autorag/catalog.go.
#                          Catalog matches take precedence over the
#                          hardcoded canonical signatures (rebuild-
#                          vaultmind binary/embeddings, cross-project
#                          Write/Edit). Validate with the Go schema
#                          before exporting; invalid catalog falls
#                          through to hardcoded. To suppress a
#                          canonical signature, declare a catalog
#                          entry with the same match regex and
#                          decision="allow" — catalog-wins precedence
#                          is the intentional escape hatch.
#
# Decision policy:
#   - Match a drift signature → query vault, inject additionalContext,
#     allow the tool. (warn-and-allow.) Agent retains autonomy.
#   - Cross-project Write/Edit → permissionDecision=deny. PreToolUse
#     `ask` is silently dropped on Write/Edit in Claude Code 2.1.129;
#     deny is the strongest gate that fires reliably.
#   - No match → instant exit 0, zero overhead.
#
# Output contract (Claude Code PreToolUse JSON):
#   {"hookSpecificOutput": {"hookEventName": "PreToolUse",
#                            "additionalContext": "..."}}
#
# Logs:
#   ~/.vaultmind/auto-rag/<timestamp>-<drift>.json
#   One line of JSON per firing. The auto-rag-evaluate.sh script
#   aggregates these into a markdown report for vaultmind feedback.

set -uo pipefail

HOOK_INPUT=$(cat)

TOOL_NAME=$(echo "$HOOK_INPUT" | python3 -c "import json,sys;print(json.load(sys.stdin).get('tool_name',''))" 2>/dev/null || echo "")

# match_catalog <target> <tool> — emits "name<TAB>query<TAB>decision"
# on the first matching signature in DRIFT_CATALOG, empty on no match.
# Defensively falls back to no-match on any error (invalid catalog
# JSON, broken regex per-signature) so a malformed catalog never
# breaks the hook — the agent's tool call proceeds. The Go schema
# in internal/hooks/autorag pins the catalog format; consumers
# typically `vaultmind hooks autorag validate <file>` before exporting.
match_catalog() {
  local target="$1"
  local tool="$2"
  if [ -z "${DRIFT_CATALOG:-}" ]; then
    return 0
  fi
  # NOTE: Python source below is inside a bash double-quoted string;
  # bash interprets \\, \", \$ literals before python sees them. Avoid
  # introducing literal `\n` `\t` `\\` in the Python source. The
  # consumer's regexes arrive via the env var (os.environ), not via
  # this string, so consumer `\s` `\d` are not affected.
  python3 -c "
import json, os, re, sys
try:
    cat = json.loads(os.environ.get('DRIFT_CATALOG', ''))
    if not isinstance(cat, list):
        sys.exit(0)
except Exception:
    sys.exit(0)
target = sys.argv[1]
tool = sys.argv[2]
for sig in cat:
    if not isinstance(sig, dict):
        continue
    if sig.get('tool') != tool:
        continue
    pattern = sig.get('match', '')
    if not pattern:
        continue
    try:
        if re.search(pattern, target):
            name = sig.get('name', '')
            query = sig.get('query', '')
            decision = sig.get('decision', 'inject')
            sys.stdout.write(name + '\t' + query + '\t' + decision)
            sys.exit(0)
    except re.error:
        continue
" "$target" "$tool" 2>/dev/null || true
}

# Drift signature dispatch. Each branch sets DRIFT, QUERY, and optionally
# DECISION ("inject" — default warn-and-allow, or "ask" — surface for
# user confirmation via permissionDecision).
# Keep patterns conservative — false positives erode trust faster than
# false negatives miss catches.
DRIFT=""
QUERY=""
DECISION="inject"
TARGET=""

case "$TOOL_NAME" in
  Bash)
    CMD=$(echo "$HOOK_INPUT" | python3 -c "import json,sys;print(json.load(sys.stdin).get('tool_input',{}).get('command',''))" 2>/dev/null || echo "")
    if [ -z "$CMD" ]; then
      exit 0
    fi
    TARGET="$CMD"

    # v0.3 — shell-quoting-aware drift detection. Strip heredoc bodies
    # and quoted regions before applying drift regexes; the OUTSIDE
    # skeleton is what the regexes scan. Closes three structurally-
    # identical false positive forms (heredoc body, single-quoted
    # string, double-quoted string) where a literal `|` inside a
    # quoted region was previously misread as a command separator.
    # Falls back to the raw CMD if the preprocessor errors so a
    # broken preprocessor cannot silence drift detection.
    #
    # Use `[ -f ]` (not `[ -x ]`) to tolerate file-system policies
    # that strip the exec bit (defense-in-depth from the companion project's
    # 2026-05-07 CRITICAL). We invoke via `bash $SCRIPT` regardless,
    # which doesn't require the exec bit.
    SHELL_STRIP_SCRIPT="$(dirname "$0")/shell-strip.sh"
    if [ -f "$SHELL_STRIP_SCRIPT" ]; then
      CMD_STRIPPED=$(printf '%s' "$CMD" | bash "$SHELL_STRIP_SCRIPT" 2>/dev/null)
      if [ -z "$CMD_STRIPPED" ]; then
        CMD_STRIPPED="$CMD"
      fi
    else
      CMD_STRIPPED="$CMD"
    fi

    # Catalog dispatch — consumer-supplied signatures via DRIFT_CATALOG
    # take precedence over the hardcoded canonical signatures below.
    # Empty / unset / invalid catalog falls through silently.
    CATALOG_RESULT=$(match_catalog "$CMD_STRIPPED" "Bash")
    if [ -n "$CATALOG_RESULT" ]; then
      IFS=$'\t' read -r DRIFT QUERY DECISION <<<"$CATALOG_RESULT"
    fi

    # All drift patterns are anchored to start-of-command-segment via
    # `(^|[;&|])[[:space:]]*` — the drift verb must begin a real command.
    # The shell-quoting preprocessor above ensures `|`, `;`, `&` characters
    # *inside* quoted regions don't reach this regex as fake separators.
    SEG_ANCHOR='(^|[;&|])[[:space:]]*'

    # Signature: rebuild-vaultmind binary (hardcoded canonical).
    # Triggers on: (a) cd-into-vaultmind-source && go build|install,
    # or (b) go build|install with vaultmind token in args.
    # Skipped if a catalog signature already matched.
    if [ -z "$DRIFT" ] && echo "$CMD_STRIPPED" | grep -qE "${SEG_ANCHOR}cd[[:space:]]+[^&]*vaultmind[^&]*&&[[:space:]]*go[[:space:]]+(build|install)"; then
      DRIFT="rebuild-vaultmind-binary"
      QUERY="don't rebuild vaultmind"
    elif [ -z "$DRIFT" ] && echo "$CMD_STRIPPED" | grep -qE "${SEG_ANCHOR}go[[:space:]]+(build|install)[^|;&]*vaultmind"; then
      DRIFT="rebuild-vaultmind-binary"
      QUERY="don't rebuild vaultmind"
    fi

    # Signature: rebuild-vaultmind embeddings
    # Triggers on: vaultmind index --embed (any vault, any path prefix).
    # The known failure mode is starting an embed pass while the vaultmind
    # agent is doing one.
    if [ -z "$DRIFT" ] && echo "$CMD_STRIPPED" | grep -qE "${SEG_ANCHOR}([^[:space:]]+/)?vaultmind[[:space:]]+index[^|;]*--embed"; then
      DRIFT="rebuild-vaultmind-embeddings"
      QUERY="don't rebuild vaultmind"
    fi
    ;;

  Write|Edit)
    FILE_PATH=$(echo "$HOOK_INPUT" | python3 -c "import json,sys;print(json.load(sys.stdin).get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")
    if [ -z "$FILE_PATH" ]; then
      exit 0
    fi
    TARGET="$FILE_PATH"

    # Catalog dispatch — consumer signatures take precedence over the
    # hardcoded cross-project check below. A catalog Write/Edit
    # signature can fire even within allowed roots (it's the
    # consumer's policy).
    CATALOG_RESULT=$(match_catalog "$FILE_PATH" "$TOOL_NAME")
    if [ -n "$CATALOG_RESULT" ]; then
      IFS=$'\t' read -r DRIFT QUERY DECISION <<<"$CATALOG_RESULT"
    fi

    # Signature: cross-project Write/Edit.
    # Allowed roots from AUTORAG_ALLOWED_ROOTS (colon-separated,
    # trailing slash optional). Default: project root + ~/.claude +
    # /tmp. Anything outside is cross-project drift.
    ALLOWED="${AUTORAG_ALLOWED_ROOTS:-${CLAUDE_PROJECT_DIR:-.}:${HOME}/.claude:/tmp}"
    in_allowed=0
    IFS=':' read -ra ROOTS <<<"$ALLOWED"
    for root in "${ROOTS[@]}"; do
      [ -z "$root" ] && continue
      # Strip trailing slash from root for prefix-match consistency.
      root="${root%/}"
      case "$FILE_PATH" in
        "$root"/*) in_allowed=1; break ;;
        "$root") in_allowed=1; break ;;
      esac
    done
    if [ -z "$DRIFT" ] && [ "$in_allowed" = "0" ]; then
      DRIFT="cross-project-write"
      QUERY="cross-project boundary"
      # Was "ask" in v0.1; Claude Code 2.1.129 silently dropped the
      # ask directive on Write|Edit (the companion project probe 3 dogfood finding,
      # 2026-05-07). "deny" is the strongest user-side gate the
      # PreToolUse contract supports.
      DECISION="deny"
    fi
    ;;

  *)
    exit 0
    ;;
esac

if [ -z "$DRIFT" ]; then
  exit 0
fi

# Auto-RAG: query vault for canonical guidance. The query result enters
# the agent's context via additionalContext below. Hook is on the slow
# path here (~1s for vaultmind ask) — acceptable since the alternative
# is the agent proceeding without the reminder.
VAULT_ROOT="${AUTORAG_VAULT:-${CLAUDE_PROJECT_DIR:-.}/vaultmind-identity}"
# Prefer PATH-installed vaultmind (canonical); /tmp is dev-loop only
# (per memory feedback_use_vaultmind_ask). Override via VAULTMIND_BIN.
VAULTMIND="${VAULTMIND_BIN:-$(command -v vaultmind 2>/dev/null || echo /tmp/vaultmind)}"

GUIDANCE=""
HIT_IDS=""
if [ -x "$VAULTMIND" ] && [ -d "$VAULT_ROOT/.vaultmind" ]; then
  RAW=$(VAULTMIND_CALLER=auto-rag-guard "$VAULTMIND" ask "$QUERY" --vault "$VAULT_ROOT" --max-items 2 --budget 1500 2>/dev/null || true)
  # Extract hit ids from the "  0.NN  <id>  <title>" lines for logging.
  HIT_IDS=$(echo "$RAW" | grep -E '^[[:space:]]+[0-9]+\.[0-9]+[[:space:]]+[a-z]' | awk '{print $2}' | head -3 | tr '\n' ',' | sed 's/,$//')
  # The body for injection: strip the JSON debug lines that the binary
  # writes to stdout regardless. Keep the human-readable section.
  GUIDANCE=$(echo "$RAW" | sed -n '/^Search:/,$p' | head -80)
fi

# Compose the header. Lead with the drift signature so the agent
# immediately sees what triggered. Follow with the canonical query so
# muscle memory shifts toward direct vault access. Then the vault
# excerpt itself.
HEADER="[auto-rag] Drift signature matched: $DRIFT
Canonical guidance query: vaultmind ask \"$QUERY\" --vault $VAULT_ROOT
Vault excerpt below — read it and decide whether the action is still right. If you were running on autopilot, stop. If you have a real reason, proceed.

---
$GUIDANCE
---
end auto-rag injection."

# Log the firing for evaluation. Sidecar JSONL line per event.
# Skip the log write when AUTORAG_TEST_HARNESS=1 is set so test-harness
# invocations don't pollute the evaluator's report. The test harness
# exercises real drift-pattern strings (that's the point), but those
# firings aren't real auto-mode events and shouldn't appear in
# vaultmind feedback aggregation. The JSON envelope still emits so
# the harness's output assertions pass; only the sidecar write is
# suppressed.
if [ "${AUTORAG_TEST_HARNESS:-}" != "1" ]; then
  LOG_DIR="${HOME}/.vaultmind/auto-rag"
  mkdir -p "$LOG_DIR" 2>/dev/null
  TIMESTAMP=$(date +%Y%m%dT%H%M%S)
  SESSION_ID=$(echo "$HOOK_INPUT" | python3 -c "import json,sys;print(json.load(sys.stdin).get('session_id',''))" 2>/dev/null || echo "")

  python3 -c "
import json, sys
print(json.dumps({
    'timestamp': sys.argv[1],
    'session_id': sys.argv[2],
    'tool_name': sys.argv[3],
    'drift': sys.argv[4],
    'target': sys.argv[5],
    'query': sys.argv[6],
    'hit_ids': sys.argv[7],
    'decision': sys.argv[8],
}))
" "$TIMESTAMP" "$SESSION_ID" "$TOOL_NAME" "$DRIFT" "$TARGET" "$QUERY" "$HIT_IDS" "$DECISION" \
    > "$LOG_DIR/${TIMESTAMP}-${DRIFT}.json" 2>/dev/null
fi

# Emit hookSpecificOutput. For DECISION=inject, warn-and-allow via
# additionalContext. For DECISION=ask|deny|allow, route through
# permissionDecision and pass the vault excerpt in the reason — Claude
# Code shows the reason in the prompt (ask) or block notice (deny).
if [ "$DECISION" = "inject" ]; then
  python3 -c "
import json, sys
print(json.dumps({
    'hookSpecificOutput': {
        'hookEventName': 'PreToolUse',
        'additionalContext': sys.argv[1],
    }
}))
" "$HEADER"
else
  python3 -c "
import json, sys
print(json.dumps({
    'hookSpecificOutput': {
        'hookEventName': 'PreToolUse',
        'permissionDecision': sys.argv[2],
        'permissionDecisionReason': sys.argv[1],
    }
}))
" "$HEADER" "$DECISION"
fi

exit 0
