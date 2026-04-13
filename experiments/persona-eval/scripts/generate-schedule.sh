#!/bin/bash
# Generate a randomized 20-session schedule for the persona eval experiment.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"

if [ -f "$SCHEDULE" ]; then
  echo "ERROR: schedule.json already exists. Delete it first to regenerate."
  exit 1
fi

SEED=$(date +%s)

python3 -c "
import json, random

seed = $SEED
random.seed(seed)

conditions = ['A'] * 7 + ['B'] * 7 + ['C'] * 6
random.shuffle(conditions)

schedule = {
    'seed': seed,
    'total_sessions': 20,
    'distribution': {'A': 7, 'B': 7, 'C': 6},
    'condition_labels': {
        'A': 'full_injection',
        'B': 'flat_paste',
        'C': 'instruction_only'
    },
    'slots': []
}

for i, cond in enumerate(conditions, 1):
    schedule['slots'].append({
        'slot': i,
        'condition': cond,
        'status': 'pending',
        'started_at': None,
        'transcript_path': None
    })

print(json.dumps(schedule, indent=2))
" > "$SCHEDULE"

echo "Schedule generated with seed $SEED -> $SCHEDULE"
