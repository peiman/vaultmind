#!/bin/bash
# Check if the persona hook fired in the last 2 minutes.
LATEST=$(ls -t ~/.vaultmind/persona-eval/*-injection.json 2>/dev/null | head -1)
if [ -z "$LATEST" ]; then
  echo "❌ No injection logs found. Hook has NEVER fired."
  exit 1
fi
AGE=$(( $(date +%s) - $(stat -f %m "$LATEST") ))
if [ "$AGE" -lt 120 ]; then
  INFO=$(python3 -c "
import json
d = json.load(open('$LATEST'))
sid = d.get('session_id', 'unknown')
chars = d.get('identity_length', 0)
print(f'session: {sid[:8]}... | identity: {chars} chars')
" 2>/dev/null || echo "")
  echo "✅ Hook fired ${AGE}s ago. $INFO"
else
  echo "❌ Hook did NOT fire. Last log is $(( AGE / 60 ))m old."
fi
