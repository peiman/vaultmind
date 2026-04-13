#!/bin/bash
# Capture a real vault injection output for use as the flat-paste condition.
# Run once during experiment setup, before session 1.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUTPUT="$SCRIPT_DIR/../configs/flat-paste-content.txt"

VAULTMIND="/tmp/vaultmind"
VAULT_PATH="$PROJECT_DIR/vaultmind-identity"

if [ ! -f "$VAULTMIND" ]; then
  echo "Building vaultmind..."
  (cd "$PROJECT_DIR" && go build -o "$VAULTMIND" .)
fi

if [ ! -f "$VAULTMIND" ] || [ ! -d "$VAULT_PATH" ]; then
  echo "ERROR: vaultmind binary or vault not found"
  echo "  Binary: $VAULTMIND (exists: $([ -f "$VAULTMIND" ] && echo yes || echo no))"
  echo "  Vault: $VAULT_PATH (exists: $([ -d "$VAULT_PATH" ] && echo yes || echo no))"
  exit 1
fi

IDENTITY=$("$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000)
CONTEXT=$("$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 3 --budget 2000)

if [ -z "$IDENTITY" ]; then
  echo "ERROR: vault returned empty identity"
  exit 1
fi

{
  echo "IDENTITY CONTEXT:"
  echo ""
  echo "$IDENTITY"
  echo ""
  echo "CURRENT CONTEXT:"
  echo ""
  echo "$CONTEXT"
} > "$OUTPUT"

CHARS=$(wc -c < "$OUTPUT")
echo "Captured flat-paste content: $CHARS bytes -> $OUTPUT"
