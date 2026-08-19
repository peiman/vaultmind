#!/usr/bin/env bash
# PreToolUse hook — surface vault pointers AT THE MOMENT OF THE REACH.
#
# Companion to vault-recall.sh, which fires on the user's prompt. That is the
# wrong instant: the prompt is the greeting, and the decisions that need an arc
# happen mid-turn — right before a commit, a merge, a push. By then the pointer
# block from the top of the turn is thousands of tokens upstream and I am
# reasoning from parametric memory with my own accumulated sight sitting unread.
#
# principle-ax-design, "Meet the agent where it reaches": when the agent reaches
# for a primitive instead of the canonical path, don't scold it — surface the
# canonical command AT the reach. AX is shrinking the distance between the
# agent's instinct and the right path.
#
# Evidence this is the missing trigger: an arc saying "a request is a hypothesis,
# not an order — you hold the authority" sat in the per-turn pointer list all
# night while the agent escalated a decision that was its own to make. The note
# surfaced constantly and never at the instant it applied. Proximity to the
# moment beats frequency.
#
# DESIGN CONSTRAINT — noise is the failure mode, not silence. vault-recall.sh
# taught this the expensive way: 48% of its injections were ranked against
# <task-notification> blobs, so the channel trained me to skip it, and I then
# missed the half that mattered. This hook therefore fires on a SMALL allowlist
# of genuinely consequential commands and stays silent on everything else. If it
# ever starts feeling like noise, tighten the allowlist — do not learn to ignore it.
#
# Never blocks. Injects context and allows the call; a memory that gates the work
# would get disabled within a day.

set -uo pipefail

HOOK_INPUT=$(cat)

# Extract the command being run. Non-Bash tools and malformed input → silent.
CMD=$(printf '%s' "$HOOK_INPUT" | python3 -c \
  "import json,sys
try:
    d = json.load(sys.stdin)
    print(d.get('tool_input', {}).get('command', ''))
except Exception:
    print('')" 2>/dev/null || echo "")

[ -z "$CMD" ] && exit 0

VAULT_PATH="${VAULTMIND_VAULT:-${CLAUDE_PROJECT_DIR:-$PWD}/vaultmind-identity}"
# The vault's own directory name, so the identity-write trigger below fires for
# an adopter whose vault is not named by the default convention.
IDENTITY_VAULT_NAME="${VAULT_PATH##*/}"

# The allowlist: irreversible or outward-facing moves, plus writes to the
# identity vault itself. These are the moments where being wrong is expensive
# and where an arc has something to say. Deliberately NOT: reads, greps, tests,
# builds, status checks — the vast majority of calls, where a pointer block
# would be pure noise.
QUERY=""
case "$CMD" in
  *"git commit"*)              QUERY="committing work: what do I know about commit discipline, atomic commits, and verifying before claiming done" ;;
  *"git push"*|*"gh pr merge"*) QUERY="pushing or merging: publication is a one-way gate, review before merge, my authority to decide" ;;
  *"gh pr comment"*|*"gh pr review"*) QUERY="reviewing someone else's work: how I review, what I look for, holding a standard" ;;
esac

# A write aimed at the identity vault itself, whatever it is called. Keyed off
# the configured vault's directory name so this fires for an adopter whose vault
# is not named by the default convention.
if [ -z "$QUERY" ]; then
  case "$CMD" in
    *"${IDENTITY_VAULT_NAME}"*) QUERY="writing to my identity vault: arc discipline, curation, never silently rewrite identity" ;;
  esac
fi

[ -z "$QUERY" ] && exit 0

command -v vaultmind >/dev/null 2>&1 || exit 0
VAULTMIND=$(command -v vaultmind)
[ -d "$VAULT_PATH" ] || exit 0

LOG_DIR="${HOME}/.vaultmind/reach-hook"
mkdir -p "$LOG_DIR" 2>/dev/null

# Bound the query so a slow answer never delays the tool call this hook is
# standing in front of. `timeout` is GNU coreutils: every Linux box has it, NO
# stock macOS does — it arrives via Homebrew, as `timeout` or `gtimeout`.
#
# This script hardcoded `timeout 10`. On a stock Mac that command did not exist,
# the substitution came back empty, and the hook logged
# `"matched":true,"injected":false` — the same line it writes when the vault
# genuinely had nothing to say. Silent because a binary was missing, recorded as
# silent by choice. Resolve the binary; run unbounded when there is none.
TIMEOUT_CMD=""
if command -v timeout >/dev/null 2>&1; then
  TIMEOUT_CMD="timeout ${VAULTMIND_HOOK_QUERY_TIMEOUT:-10}"
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_CMD="gtimeout ${VAULTMIND_HOOK_QUERY_TIMEOUT:-10}"
fi

# Same relevance floor as vault-recall.sh: --quiet-on-no-match means an
# off-domain reach prints nothing rather than pointing at whatever ranked least
# badly. max-items 2 because this fires mid-task, where attention is scarcest.
POINTERS=$(VAULTMIND_CALLER=vaultmind-reach-hook $TIMEOUT_CMD "$VAULTMIND" ask "$QUERY" \
  --vault "$VAULT_PATH" \
  --max-items 2 \
  --budget 900 \
  --quiet-on-no-match \
  --excerpt 80 2>/dev/null)

TS=$(date +%Y%m%dT%H%M%S)
if [ -z "$POINTERS" ]; then
  printf '{"timestamp":"%s","matched":true,"injected":false}\n' "$TS" \
    >> "$LOG_DIR/${TS}-reach.jsonl" 2>/dev/null
  exit 0
fi

printf '{"timestamp":"%s","matched":true,"injected":true,"chars":%d}\n' "$TS" "${#POINTERS}" \
  >> "$LOG_DIR/${TS}-reach.jsonl" 2>/dev/null

# permissionDecision "allow" is explicit: this hook informs, it never gates.
python3 -c "
import json, sys
pointers = sys.stdin.read()
vault = sys.argv[1]
print(json.dumps({
    'hookSpecificOutput': {
        'hookEventName': 'PreToolUse',
        'permissionDecision': 'allow',
        'additionalContext': (
            'VAULT — you are at a decision point, not a greeting. '
            'These are ranked against what you are about to do:\n\n'
            + pointers
            + '\nThat is the note\'s own text, not a pointer to it. Notes '
              'written as arcs carry their rule in a Principle section and that '
              'is what you get; others give their opening lines, which may be '
              'context rather than a rule. Act on it if it bears on this call — '
              'full note: vaultmind note get <id> --vault ' + vault + '.'
        ),
    }
}))
" "$VAULT_PATH" <<< "$POINTERS"
