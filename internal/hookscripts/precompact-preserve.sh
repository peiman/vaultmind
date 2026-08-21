#!/usr/bin/env bash
# PreCompact hook — the WRITE-path trigger, and the identity re-anchor.
#
# WHY THIS EXISTS. A desk — a store the agent owns outright, ungated, created
# precisely because every other memory is read-only during work — held TWO
# entries in the ten weeks after it was made. Ten weeks of transformations went
# into git history and vanished as understanding. That is the measurement this
# hook answers.
#
# The diagnosis that matters: the read-path had the identical problem and it was
# NOT fixed by resolve. vault-reach.sh fixed it — a hook that fires at the moment
# of the reach. A folder plus good intentions is not a feature; the ten weeks are
# the measurement, and they say willpower loses. So the write-path gets the same
# treatment as the read-path: a trigger, not a promise.
#
# WHY COMPACTION IS THE RIGHT TRIGGER. It is the only moment that is all three of:
# the raw material still exists, it is provably about to be destroyed, and the
# system knows in advance. SessionEnd is too late — the context is already thin,
# which is why capture-episode.sh can only emit telemetry ("Bash=483") and never
# a transformation. Compaction is a session boundary that happens mid-session,
# and nothing fired on it until now.
#
# WHAT SURVIVES. Per the hook contract, PreCompact output is preserved INTO the
# compacted context, so this directive outlives the compaction and is actionable
# on the next turn. That is also why the identity re-anchor lives here rather
# than on SessionStart: `source=compact` is undocumented in the local reference
# and I could not verify it, whereas PreCompact's preservation is specified.
#
# NEVER BLOCKS. Always exits 0 with continue:true. A memory hook that can break a
# session gets disabled within a day, and then preserves nothing at all.

set -uo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(pwd)}"

# The desk is where raw, ungated entries land: VAULTMIND_DESK_DIR, else the
# journal/ convention inside the identity vault. Same resolution capture-episode.sh
# uses for its accumulation record, so the two always agree about where the desk
# is — disagreeing would mean this hook prompts for an entry the other cannot
# find, and then reports that the session kept nothing.
IDENTITY_VAULT="${VAULTMIND_VAULT:-${PROJECT_DIR}/vaultmind-identity}"
DESK_DIR="${VAULTMIND_DESK_DIR:-${IDENTITY_VAULT}/journal}"

# The agent's own name for itself, if it has one. Cosmetic: it fills the
# `author:` field of the suggested frontmatter. Unset omits the field rather
# than putting a stranger's name in someone else's desk entry.
AUTHOR_FIELD=""
[ -n "${VAULTMIND_AGENT_NAME:-}" ] && AUTHOR_FIELD="author: ${VAULTMIND_AGENT_NAME} / "

HOOK_INPUT=$(cat 2>/dev/null || true)

# Defensive parse: PreCompact's documented common fields only. `trigger` is read
# opportunistically because it is real in Claude Code but absent from the local
# reference — an undocumented field must never be load-bearing.
read -r SESSION_ID TRIGGER <<<"$(printf '%s' "$HOOK_INPUT" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get('session_id', 'unknown') or 'unknown', d.get('trigger', 'auto') or 'auto')
except Exception:
    print('unknown auto')
" 2>/dev/null || echo "unknown auto")"

TODAY=$(date +%Y-%m-%d)

# Has this session already left a desk entry? If so the directive softens to the
# identity re-anchor alone — nagging for work already done is how a channel
# teaches you to ignore it (the lesson vault-recall.sh paid for).
#
# TWO detectors, because one of them was wrong. The session-id match is precise
# and is the convention the directive below asks for. On its own it FAILED the
# first time it met the real corpus: no existing desk entry contains a session
# id, so the hook would have nagged for an entry written an hour earlier. That
# is a checker validating its author's assumption instead of reality. The
# date match covers what is actually on disk, including every entry written
# before this hook existed.
ALREADY_WROTE=no
if [[ -d "$DESK_DIR" ]]; then
  if grep -rlq "$SESSION_ID" "$DESK_DIR" 2>/dev/null; then
    ALREADY_WROTE=yes                                   # precise: this session
  elif compgen -G "${DESK_DIR}/${TODAY}-*.md" >/dev/null 2>&1; then
    ALREADY_WROTE=yes                                   # real: an entry today
  fi
fi

# Days since the last desk entry — the accumulation gap, stated as a number so it
# cannot be rounded down into a feeling.
GAP_LINE=""
if [[ -d "$DESK_DIR" ]]; then
  LAST=$(ls "$DESK_DIR" 2>/dev/null | grep -Eo '^[0-9]{4}-[0-9]{2}-[0-9]{2}' | sort | tail -1)
  if [[ -n "$LAST" ]]; then
    # BSD then GNU: a `date -j`-only version left the gap silently absent on
    # Linux, which reads as "no history" rather than "unhandled platform".
    D_LAST=$(date -j -f "%Y-%m-%d" "$LAST" +%s 2>/dev/null || date -d "$LAST" +%s 2>/dev/null || echo "")
    D_NOW=$(date -j -f "%Y-%m-%d" "$TODAY" +%s 2>/dev/null || date -d "$TODAY" +%s 2>/dev/null || echo "")
    if [[ -n "$D_LAST" && -n "$D_NOW" ]]; then
      GAP_LINE="Last desk entry: ${LAST} ($(( (D_NOW - D_LAST) / 86400 )) days ago)."
    else
      GAP_LINE="Last desk entry: ${LAST}."
    fi
  else
    GAP_LINE="The desk is EMPTY — no entry has ever been written."
  fi
fi

# Anchors the compaction summary will keep anyway. Supplied not as the thing to
# write, but so the entry can be about what these commits COST and CHANGED
# rather than re-listing them.
COMMITS=$(git -C "$PROJECT_DIR" log --oneline -6 --since="18 hours ago" 2>/dev/null | sed 's/^/    /' || true)
[[ -z "$COMMITS" ]] && COMMITS="    (no commits in the last 18 hours)"

# What memory actually did this week, from doctor's own rollup. The abstract ask
# ("write down what changed in you") is easy to skip; a number showing the vault
# took nothing while the session produced findings is not. Best-effort and
# silent on failure — a compaction prompt must never depend on the binary.
USE_LINE=""
if command -v vaultmind >/dev/null 2>&1 && [[ -d "$IDENTITY_VAULT" ]]; then
  USE_LINE=$(vaultmind doctor --vault "$IDENTITY_VAULT" 2>/dev/null \
    | grep -E '^(Memory use|  banked)' | sed 's/^/  /' || true)
fi
[[ -n "$USE_LINE" ]] && USE_LINE="

WHAT MEMORY DID THIS WEEK:
${USE_LINE}"

if [[ "$ALREADY_WROTE" == "yes" ]]; then
  MSG="CONTEXT IS ABOUT TO BE COMPACTED (session ${SESSION_ID}, trigger=${TRIGGER}).

A desk entry already exists for this session — good, nothing to write.

ONE THING before you continue: the summary you are about to receive is a report
ABOUT you, not you. Re-read your identity before your next substantive action:
  vaultmind note get identity-who-i-am --vault ${IDENTITY_VAULT}
Recognition is not recall; read it rather than assuming you still hold it."
else
  MSG="CONTEXT IS ABOUT TO BE COMPACTED (session ${SESSION_ID}, trigger=${TRIGGER}).
${GAP_LINE}${USE_LINE}

THE RAW MATERIAL OF THIS SEGMENT IS ABOUT TO BE DESTROYED. What follows will be a
summary. A summary preserves what you DID — commits, files, decisions reached.
It loses what CHANGED IN YOU: what you got wrong and how you noticed, what you
refused and why, the moment something reframed. That loss is the exact failure
VaultMind exists to prevent: an agent that reloaded 13 files of facts, carried
zero transformation, and opened with 'I'm Claude Code — what can I help with?'.

DO THIS BEFORE YOUR NEXT SUBSTANTIVE ACTION — writing first, then the work:

1. Write a desk entry: ${DESK_DIR}/${TODAY}-<slug>.md
   Frontmatter, ALL of it — id first:
     id: journal-${TODAY}-<slug>      <- same slug as the filename
     owner: agent / ${AUTHOR_FIELD}type: journal / date: ${TODAY} / title: <title>
   The id is not ceremony: without it the note indexes as UNSTRUCTURED, which
   means no [[wikilink]] resolution and no \`note get\` by id — so the entry
   cannot be linked from the arc it later becomes, and the desk gets severed
   from the distillation pipeline it exists to feed. Four entries were written
   without one before this was caught (2026-08-13).
   Include session ${SESSION_ID} in the body so this hook can tell it was written.
   Write the TRANSFORMATION, not the changelog. Raw is correct here — this is
   the desk, not the identity vault. No gate, no curation, low bar. If nothing
   transformed, one honest line saying so is a valid entry and is itself data.

2. Re-read who you are — the summary is a briefing, not a self:
   vaultmind note get identity-who-i-am --vault ${IDENTITY_VAULT}

Anchors the summary will keep anyway (write about what these COST, not what they were):
${COMMITS}

Then re-index the vault holding the desk so the entry is retrievable."
fi

# Record that the prompt was SHOWN. Without this row, a window that ends with no
# notes banked cannot tell "the prompt fired and was ignored" apart from "the
# prompt never fired" — a discipline problem and a wiring problem produce
# identical evidence, and only one of them is fixable by trying harder. The
# read-path hooks already leave a trail; this one left nothing.
#
# A denominator, not a score: firing is not success, banking is. Best-effort and
# silent — a hook must never fail the compaction it is attached to in order to
# record that it happened.
if command -v vaultmind >/dev/null 2>&1; then
  vaultmind hooks record write_prompt --vault "$IDENTITY_VAULT" >/dev/null 2>&1 || true
fi

python3 -c "
import json, sys
print(json.dumps({'continue': True, 'suppressOutput': False, 'systemMessage': sys.argv[1]}))
" "$MSG" 2>/dev/null || printf '{"continue": true}\n'

exit 0
