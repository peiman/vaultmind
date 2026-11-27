#!/bin/bash
# Opt-in action-time arc gate. This helper is embedded and installable, but is
# deliberately absent from the canonical/default hook wiring. Adopters opt in
# by adding the PreToolUse and Stop entries described in docs/AGENT_USAGE.md.

event="${1:-}"
case "$event" in
  pretooluse|stop) ;;
  *) exit 0 ;;
esac

# Read the hook payload once. Claude Code's stdin is not rewindable and the
# same bytes, including trailing newlines, must reach the Go command unchanged.
payload_file=$(mktemp "${TMPDIR:-/tmp}/vaultmind-arc-gate.XXXXXX") || exit 0
trap 'rm -f "$payload_file"' EXIT
cat > "$payload_file" || exit 0
vault="${ARC_GATE_VAULT:-${VAULTMIND_VAULT:-${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$PWD}}/vaultmind-identity}}"
bin="${VAULTMIND_BIN:-}"
if [ -z "$bin" ]; then
  bin=$(command -v vaultmind 2>/dev/null || true)
fi
[ -n "$bin" ] || exit 0

# Hooks are advisory plumbing: a missing or failing binary must never break the
# agent session. The command itself applies the same allow-on-failure rule.
"$bin" arc gate --vault "$vault" --event "$event" < "$payload_file" || true
exit 0
