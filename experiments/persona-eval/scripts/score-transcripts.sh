#!/bin/bash
# Score experiment transcripts using a configurable LLM rater.
# Usage: score-transcripts.sh --llm openai/gpt-4o
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
python3 "$SCRIPT_DIR/score_transcripts.py" "$@"
