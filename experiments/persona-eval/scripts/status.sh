#!/usr/bin/env bash
# Show persona-eval experiment progress at a glance.
#
# Usage: bash experiments/persona-eval/scripts/status.sh
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"

if [ ! -f "$SCHEDULE" ]; then
  echo "No schedule.json found. Generate one with:"
  echo "  bash experiments/persona-eval/scripts/generate-schedule.sh"
  exit 0
fi

python3 - "$SCHEDULE" <<'PY'
import json, sys
s = json.load(open(sys.argv[1]))
slots = s['slots']
total = s['total_sessions']

complete = [sl for sl in slots if sl['status'] == 'complete']
started = [sl for sl in slots if sl['status'] == 'started']
pending = [sl for sl in slots if sl['status'] == 'pending']

print(f"Persona-eval progress: {len(complete)}/{total} complete")
print()

# Per-condition breakdown
counts = {'A': {'done': 0, 'total': 0}, 'B': {'done': 0, 'total': 0}, 'C': {'done': 0, 'total': 0}}
for sl in slots:
    counts[sl['condition']]['total'] += 1
    if sl['status'] == 'complete':
        counts[sl['condition']]['done'] += 1
labels = s.get('condition_labels', {})
for c in ('A', 'B', 'C'):
    label = labels.get(c, '')
    print(f"  Condition {c} ({label:>17}): {counts[c]['done']}/{counts[c]['total']}")

print()
if started:
    sl = started[0]
    print(f"Currently started: slot {sl['slot']} (condition {sl['condition']})")
elif pending:
    sl = pending[0]
    print(f"Next pending:      slot {sl['slot']} (condition {sl['condition']})")
else:
    print("All slots complete. Run scoring:")
    print("  bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o")
    sys.exit(0)

print()
print("Open a new Claude Code session in this directory to advance.")
PY
