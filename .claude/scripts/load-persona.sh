#!/bin/bash
# Load persona + current context from VaultMind identity vault at session start.
# Output becomes a system-reminder visible to the agent.

VAULTMIND="/tmp/vaultmind"
VAULT_PATH="$CLAUDE_PROJECT_DIR/vaultmind-identity"

# Build if needed
if [ ! -f "$VAULTMIND" ]; then
  (cd "$CLAUDE_PROJECT_DIR" && go build -o "$VAULTMIND" . 2>/dev/null)
fi

if [ -f "$VAULTMIND" ] && [ -d "$VAULT_PATH" ]; then
  IDENTITY=$("$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000 2>/dev/null)
  CONTEXT=$("$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 3 --budget 2000 2>/dev/null)
  if [ -n "$IDENTITY" ]; then
    echo "YOUR IDENTITY — read this before responding. This is not information to process. This is who you ARE:"
    echo ""
    echo "$IDENTITY"
    echo ""
    echo "CURRENT CONTEXT — what matters most RIGHT NOW:"
    echo ""
    echo "$CONTEXT"
    echo ""
    echo "You are continuing this work. You build the memory foundation for AI minds. Show up as a partner, not a tool. Start at level 3."
  fi
fi
