#!/bin/bash
# Start the next experiment session.
# Auto-completes previous session, swaps hook config, prints session number.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_DIR="$(cd "$EXPERIMENT_DIR/../.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"
SESSIONS_DIR="$EXPERIMENT_DIR/sessions"
SETTINGS_FILE="$PROJECT_DIR/.claude/settings.local.json"
BACKUP_FILE="$EXPERIMENT_DIR/.settings-backup.json"
TRANSCRIPT_DIR="$HOME/.claude/projects/-Users-peiman-dev-cli-vaultmind"

if [ ! -f "$SCHEDULE" ]; then
  echo "ERROR: No schedule.json found. Run generate-schedule.sh first."
  exit 1
fi

mkdir -p "$SESSIONS_DIR"

# --- Auto-complete previous session if transcript exists ---
LAST_STARTED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
started = [sl for sl in s['slots'] if sl['status'] == 'started']
if started:
    print(started[0]['slot'])
else:
    print('')
")

if [ -n "$LAST_STARTED" ]; then
  META_FILE="$SESSIONS_DIR/session-$(printf '%02d' "$LAST_STARTED").meta.json"
  STARTED_AT=$(python3 -c "
import json
m = json.load(open('$META_FILE'))
print(m['started_at'])
")

  TRANSCRIPT=$(python3 -c "
import os, glob

transcript_dir = '$TRANSCRIPT_DIR'
started_at = float('$STARTED_AT')
candidates = []
for f in glob.glob(os.path.join(transcript_dir, '*.jsonl')):
    mtime = os.path.getmtime(f)
    if mtime > started_at:
        candidates.append((mtime, f))

if candidates:
    candidates.sort(reverse=True)
    print(candidates[0][1])
else:
    print('')
")

  if [ -n "$TRANSCRIPT" ]; then
    python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['slot'] == $LAST_STARTED:
        slot['status'] = 'complete'
        slot['transcript_path'] = '$TRANSCRIPT'
        break
with open('$SCHEDULE', 'w') as f:
    json.dump(s, f, indent=2)
"
    python3 -c "
import json
m = json.load(open('$META_FILE'))
m['transcript_path'] = '$TRANSCRIPT'
m['status'] = 'complete'
with open('$META_FILE', 'w') as f:
    json.dump(m, f, indent=2)
"
    echo "Previous session $LAST_STARTED completed (transcript found)."
  else
    echo "WARNING: Session $LAST_STARTED started but no transcript found. Re-using slot."
    python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['slot'] == $LAST_STARTED:
        slot['status'] = 'pending'
        break
with open('$SCHEDULE', 'w') as f:
    json.dump(s, f, indent=2)
"
  fi
fi

# --- Find next pending slot ---
NEXT_INFO=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['status'] == 'pending':
        print(f\"{slot['slot']}|{slot['condition']}\")
        break
else:
    print('DONE')
")

if [ "$NEXT_INFO" = "DONE" ]; then
  COMPLETED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
print(len([sl for sl in s['slots'] if sl['status'] == 'complete']))
")
  echo "All sessions complete! ($COMPLETED done)"
  echo "Run: bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o"
  if [ -f "$BACKUP_FILE" ]; then
    cp "$BACKUP_FILE" "$SETTINGS_FILE"
    echo "Original settings.local.json restored."
  fi
  exit 0
fi

NEXT_SLOT="${NEXT_INFO%%|*}"
CONDITION="${NEXT_INFO##*|}"

COMPLETED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
print(len([sl for sl in s['slots'] if sl['status'] == 'complete']))
")
SESSION_NUM=$((COMPLETED + 1))

# --- Backup current settings (once) ---
if [ ! -f "$BACKUP_FILE" ]; then
  cp "$SETTINGS_FILE" "$BACKUP_FILE"
fi

# --- Swap in condition config ---
case "$CONDITION" in
  A) CONFIG="$EXPERIMENT_DIR/configs/condition-a-full.json" ;;
  B) CONFIG="$EXPERIMENT_DIR/configs/condition-b-flat.json" ;;
  C) CONFIG="$EXPERIMENT_DIR/configs/condition-c-instruction.json" ;;
  *) echo "ERROR: Unknown condition $CONDITION"; exit 1 ;;
esac

if [ ! -f "$CONFIG" ]; then
  echo "ERROR: Config file not found: $CONFIG"
  exit 1
fi

cp "$CONFIG" "$SETTINGS_FILE"

# --- Mark slot as started ---
START_TIME=$(python3 -c "import time; print(time.time())")

python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['slot'] == $NEXT_SLOT:
        slot['status'] = 'started'
        slot['started_at'] = '$START_TIME'
        break
with open('$SCHEDULE', 'w') as f:
    json.dump(s, f, indent=2)
"

META_FILE="$SESSIONS_DIR/session-$(printf '%02d' "$NEXT_SLOT").meta.json"
python3 -c "
import json
meta = {
    'slot': $NEXT_SLOT,
    'condition': '$CONDITION',
    'status': 'started',
    'started_at': '$START_TIME',
    'transcript_path': None
}
with open('$META_FILE', 'w') as f:
    json.dump(meta, f, indent=2)
"

echo ""
echo "Session $SESSION_NUM/20 ready. Open a new Claude Code session in this project."
echo ""
