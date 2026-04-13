#!/bin/bash
# Inject hardcoded flat-paste content for condition B.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FLAT_FILE="$SCRIPT_DIR/../configs/flat-paste-content.txt"

if [ -f "$FLAT_FILE" ]; then
  cat "$FLAT_FILE"
else
  echo "ERROR: flat-paste-content.txt not found. Run capture-flat-paste.sh first." >&2
fi
