#!/usr/bin/env bash
# Auto-advance the persona-eval schedule on every Claude Code SessionStart.
#
# Safe to run on every startup:
#   - no schedule.json → silent no-op
#   - all slots complete → prints "EXPERIMENT COMPLETE" once (restoring
#     backups), then silent on subsequent runs
#   - a "started" slot has a transcript newer than its snapshot → auto-complete
#     that slot, advance to the next pending, swap settings.local.json to the
#     next condition, and print "EXPERIMENT: slot N/M ready"
#   - a "started" slot has no new transcript yet → prints "EXPERIMENT: slot
#     N/M in progress" and exits. The NEXT Claude Code startup will find the
#     transcript produced by this session and advance.
#
# The current session's settings.local.json is already loaded by Claude Code
# before this hook runs, so swapping here affects the NEXT session only.
# Peiman is told to close + re-open Claude Code whenever an advance occurs.
#
# AUTO_ADVANCE_TRANSCRIPT_DIR overrides the default transcript dir (for tests).

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_DIR="$(cd "$EXPERIMENT_DIR/../.." && pwd)"

SCHEDULE="$EXPERIMENT_DIR/schedule.json"
SESSIONS_DIR="$EXPERIMENT_DIR/sessions"
CONFIGS_DIR="$EXPERIMENT_DIR/configs"
SETTINGS_FILE="$PROJECT_DIR/.claude/settings.local.json"
CLAUDE_MD="$PROJECT_DIR/CLAUDE.md"
BACKUP_FILE="$EXPERIMENT_DIR/.settings-backup.json"
CLAUDE_MD_BACKUP="$EXPERIMENT_DIR/.claude-md-backup.md"
TRANSCRIPT_DIR="${AUTO_ADVANCE_TRANSCRIPT_DIR:-$HOME/.claude/projects/-Users-peiman-dev-cli-vaultmind}"
FIND_TRANSCRIPT="$SCRIPT_DIR/lib/find-new-transcript.py"

# notify: surface a user-visible macOS notification.
# Hook stdout goes to the agent's context, not the user's terminal, so without
# this the user never sees the experiment state change.
# No-op on non-macOS or when AUTO_ADVANCE_NO_NOTIFY is set (used by tests).
notify() {
  local title="$1" subtitle="$2" body="$3"
  [ -n "${AUTO_ADVANCE_NO_NOTIFY:-}" ] && return 0
  command -v osascript >/dev/null 2>&1 || return 0
  osascript -e "display notification \"$body\" with title \"$title\" subtitle \"$subtitle\" sound name \"Glass\"" >/dev/null 2>&1 || true
}

# Read the current session's transcript_path from stdin JSON if present.
# Claude Code passes {"session_id","transcript_path","hook_event_name",...} on
# stdin for SessionStart hooks. We exclude that path from new-transcript
# detection so we never mistake the active session for a completed prior one.
CURRENT_TRANSCRIPT=""
if [ ! -t 0 ]; then
  STDIN_JSON=$(cat)
  if [ -n "$STDIN_JSON" ]; then
    CURRENT_TRANSCRIPT=$(printf '%s' "$STDIN_JSON" | python3 -c '
import json, sys
try:
    print(json.loads(sys.stdin.read()).get("transcript_path", "") or "")
except Exception:
    print("")
' 2>/dev/null || echo "")
  fi
fi

# --- No-op #1: no schedule ---
if [ ! -f "$SCHEDULE" ]; then
  exit 0
fi

total=$(python3 -c "import json;print(json.load(open('$SCHEDULE'))['total_sessions'])")
completed=$(python3 -c "import json;print(len([s for s in json.load(open('$SCHEDULE'))['slots'] if s['status']=='complete']))")

# --- No-op #2: all slots complete → restore once, then silent ---
if [ "$completed" -ge "$total" ]; then
  if [ -f "$BACKUP_FILE" ]; then
    cp "$BACKUP_FILE" "$SETTINGS_FILE" 2>/dev/null || true
    rm "$BACKUP_FILE"
  fi
  if [ -f "$CLAUDE_MD_BACKUP" ]; then
    cp "$CLAUDE_MD_BACKUP" "$CLAUDE_MD" 2>/dev/null || true
    rm "$CLAUDE_MD_BACKUP"
  fi
  notify "VaultMind experiment" "All sessions complete" "$completed/$total done — run score-transcripts.sh"
  echo "EXPERIMENT COMPLETE ($completed/$total). Run: bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o"
  exit 0
fi

# --- Identify the currently-started slot (at most one) ---
started_slot=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
started = [sl for sl in s['slots'] if sl['status']=='started']
print(started[0]['slot'] if started else '')
")

# --- Auto-complete a started slot if a new transcript has appeared ---
advanced="no"
if [ -n "$started_slot" ]; then
  meta_file="$SESSIONS_DIR/session-$(printf '%02d' "$started_slot").meta.json"
  snapshot_file="$SESSIONS_DIR/session-$(printf '%02d' "$started_slot").snapshot.txt"
  if [ -n "$CURRENT_TRANSCRIPT" ]; then
    transcript=$("$FIND_TRANSCRIPT" "$TRANSCRIPT_DIR" "$snapshot_file" --exclude "$CURRENT_TRANSCRIPT")
  else
    transcript=$("$FIND_TRANSCRIPT" "$TRANSCRIPT_DIR" "$snapshot_file")
  fi
  if [ -n "$transcript" ]; then
    # Pass transcript path + slot via env so a path containing single quotes
    # cannot break out of a Python string literal (G201/shell-injection class).
    SCHEDULE_PATH="$SCHEDULE" TRANSCRIPT_PATH="$transcript" SLOT="$started_slot" python3 - <<'PY'
import json, os
p = os.environ['SCHEDULE_PATH']
slot = int(os.environ['SLOT'])
transcript = os.environ['TRANSCRIPT_PATH']
s = json.load(open(p))
for sl in s['slots']:
    if sl['slot'] == slot:
        sl['status'] = 'complete'
        sl['transcript_path'] = transcript
        break
json.dump(s, open(p, 'w'), indent=2)
PY
    if [ -f "$meta_file" ]; then
      META_PATH="$meta_file" TRANSCRIPT_PATH="$transcript" python3 - <<'PY'
import json, os
p = os.environ['META_PATH']
transcript = os.environ['TRANSCRIPT_PATH']
m = json.load(open(p))
m['status'] = 'complete'
m['transcript_path'] = transcript
json.dump(m, open(p, 'w'), indent=2)
PY
    fi
    advanced="yes"
  else
    # Started but no completed prior transcript — THIS session is the work session.
    started_condition=$(python3 -c "
import json
s=json.load(open('$SCHEDULE'))
for sl in s['slots']:
    if sl['slot']==$started_slot:
        print(sl['condition']); break
" 2>/dev/null)
    notify "VaultMind slot $started_slot/$total ACTIVE" "Condition $started_condition — work session" "Do natural work, then close Claude Code."
    echo "EXPERIMENT slot $started_slot/$total ACTIVE (condition $started_condition). This is your work session — do whatever you'd normally do on this project, then close Claude Code when you're done. The next Claude Code startup will auto-advance."
    exit 0
  fi
fi

# --- Pick the next pending slot ---
next_info=$(python3 -c "
import json
s=json.load(open('$SCHEDULE'))
for sl in s['slots']:
    if sl['status']=='pending':
        print(f\"{sl['slot']}|{sl['condition']}\")
        break
else:
    print('DONE')
")

if [ "$next_info" = "DONE" ]; then
  completed=$(python3 -c "import json;print(len([s for s in json.load(open('$SCHEDULE'))['slots'] if s['status']=='complete']))")
  if [ -f "$BACKUP_FILE" ]; then
    cp "$BACKUP_FILE" "$SETTINGS_FILE" 2>/dev/null || true
    rm "$BACKUP_FILE"
  fi
  if [ -f "$CLAUDE_MD_BACKUP" ]; then
    cp "$CLAUDE_MD_BACKUP" "$CLAUDE_MD" 2>/dev/null || true
    rm "$CLAUDE_MD_BACKUP"
  fi
  notify "VaultMind experiment" "All sessions complete" "$completed/$total done — run score-transcripts.sh"
  echo "EXPERIMENT COMPLETE ($completed/$total). Run: bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o"
  exit 0
fi

next_slot="${next_info%%|*}"
next_condition="${next_info##*|}"

# --- Backup originals once ---
if [ ! -f "$BACKUP_FILE" ] && [ -f "$SETTINGS_FILE" ]; then
  cp "$SETTINGS_FILE" "$BACKUP_FILE"
fi
if [ ! -f "$CLAUDE_MD_BACKUP" ] && [ -f "$CLAUDE_MD" ]; then
  cp "$CLAUDE_MD" "$CLAUDE_MD_BACKUP"
  python3 - "$CLAUDE_MD" <<'PY'
import sys
p = sys.argv[1]
content = open(p).read()
content = content.replace(
  "A SessionStart hook loads it automatically, but if it didn't fire, run: `/tmp/vaultmind ask \"who am I\" --vault vaultmind-identity --max-items 8 --budget 6000` and read the output.",
  "A SessionStart hook loads it automatically.",
)
open(p, "w").write(content)
PY
fi

# --- Swap settings.local.json for the next condition ---
case "$next_condition" in
  A) config="$CONFIGS_DIR/condition-a-full.json" ;;
  B) config="$CONFIGS_DIR/condition-b-flat.json" ;;
  C) config="$CONFIGS_DIR/condition-c-instruction.json" ;;
  *) echo "auto-advance: unknown condition '$next_condition'" >&2; exit 1 ;;
esac
if [ ! -f "$config" ]; then
  echo "auto-advance: missing config file $config" >&2
  exit 1
fi
cp "$config" "$SETTINGS_FILE"

# --- Mark next slot as started with timestamp + snapshot ---
start_time=$(python3 -c "import time;print(time.time())")
python3 -c "
import json
p='$SCHEDULE'
s=json.load(open(p))
for sl in s['slots']:
    if sl['slot']==$next_slot:
        sl['status']='started'
        sl['started_at']=$start_time
        break
json.dump(s,open(p,'w'),indent=2)
"
mkdir -p "$SESSIONS_DIR"
meta_file="$SESSIONS_DIR/session-$(printf '%02d' "$next_slot").meta.json"
python3 -c "
import json
open('$meta_file','w').write(json.dumps({
  'slot': $next_slot,
  'condition': '$next_condition',
  'status': 'started',
  'started_at': $start_time,
  'transcript_path': None,
}, indent=2))
"
snapshot_file="$SESSIONS_DIR/session-$(printf '%02d' "$next_slot").snapshot.txt"
python3 -c "
import glob, os
files=sorted(glob.glob(os.path.join('$TRANSCRIPT_DIR','*.jsonl')))
open('$snapshot_file','w').write(('\n'.join(files)+'\n') if files else '')
"

# --- Banner ---
if [ "$advanced" = "yes" ]; then
  prev=$((next_slot - 1))
  notify "VaultMind slot $prev COMPLETED" "Slot $next_slot/$total READY (condition $next_condition)" "CLOSE + REOPEN Claude Code to begin slot $next_slot."
  echo "EXPERIMENT slot $prev/$total COMPLETED. Slot $next_slot/$total READY (condition $next_condition swapped in). CLOSE this Claude Code window and REOPEN to begin slot $next_slot."
else
  notify "VaultMind slot $next_slot/$total READY" "Condition $next_condition swapped in" "CLOSE + REOPEN Claude Code to begin slot $next_slot."
  echo "EXPERIMENT slot $next_slot/$total READY (condition $next_condition swapped in). CLOSE this Claude Code window and REOPEN to begin slot $next_slot."
fi
