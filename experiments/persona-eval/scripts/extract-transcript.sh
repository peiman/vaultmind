#!/bin/bash
# Extract readable USER/ASSISTANT turns from a Claude Code JSONL transcript.
# Usage: extract-transcript.sh <path-to-jsonl>
set -e

if [ -z "$1" ] || [ ! -f "$1" ]; then
  echo "Usage: extract-transcript.sh <path-to-jsonl>" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
python3 "$SCRIPT_DIR/extract_transcript.py" "$1"
