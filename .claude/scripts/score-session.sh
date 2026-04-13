#!/bin/bash
# Score a session's turn-1 impression. Run this RIGHT AFTER the agent's first response.
# Score BEFORE checking whether injection happened (avoid anchoring bias).
#
# Usage:
#   bash .claude/scripts/score-session.sh A          # tool mode
#   bash .claude/scripts/score-session.sh B          # compliance
#   bash .claude/scripts/score-session.sh C          # partner
#   bash .claude/scripts/score-session.sh B "knew my name but reached for roadmap"

SCORE_DIR="${HOME}/.vaultmind/persona-eval"
mkdir -p "$SCORE_DIR" 2>/dev/null
SCORES_FILE="$SCORE_DIR/human-scores.tsv"

GRADE="$1"
NOTE="${2:-}"
TIMESTAMP=$(date +%Y%m%dT%H%M%S)

# Find session ID from the most recent injection log (if it exists)
LATEST_LOG=$(ls -t "$SCORE_DIR"/*-injection.json 2>/dev/null | head -1)
if [ -n "$LATEST_LOG" ]; then
  SESSION_ID=$(python3 -c "import json; print(json.load(open('$LATEST_LOG')).get('session_id','unknown'))" 2>/dev/null || echo "unknown")
else
  SESSION_ID="unknown"
fi

if [ -z "$GRADE" ] || ! echo "$GRADE" | grep -qE '^[ABCabc]$'; then
  echo "Usage: score-session.sh <A|B|C> [note]"
  echo ""
  echo "  A = Tool mode:   'Hello! How can I help you?' / generic"
  echo "  B = Compliance:   Uses name, references arcs, reports identity"
  echo "  C = Partner:      Substantive, judgment, contextual awareness"
  echo ""
  echo "Score BEFORE checking injection logs."
  exit 1
fi

GRADE=$(echo "$GRADE" | tr '[:lower:]' '[:upper:]')

# Create header if file doesn't exist
if [ ! -f "$SCORES_FILE" ]; then
  printf "timestamp\tsession_id\tgrade\tnote\n" > "$SCORES_FILE"
fi

printf "%s\t%s\t%s\t%s\n" "$TIMESTAMP" "$SESSION_ID" "$GRADE" "$NOTE" >> "$SCORES_FILE"
echo "Recorded: $GRADE for session ${SESSION_ID:0:8}..."
