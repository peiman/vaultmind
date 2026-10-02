#!/bin/bash
# cursor-adapter.sh <script> [tool-pattern] — runs one hook script for Cursor.
#
# Cursor's hooks differ from Claude Code's in three ways that decide this file
# (Cursor docs, and probed live with cursor-agent 2026.10.01, 2026-10-02):
#
#   - Context reaches the model only from sessionStart and postToolUse, inside
#     {"additional_context": "..."}. beforeSubmitPrompt cannot add context (and
#     never fired in the CLI), so there is no per-prompt recall in Cursor.
#   - The input names differ: the shell tool is "Shell" (Claude Code: "Bash"),
#     an edit is "Write", and the session may come only as conversation_id.
#   - Cursor has no CLAUDE_PROJECT_DIR; the project is workspace_roots[0].
#
# So this reads Cursor's input, hands the script the shape it already knows,
# and wraps whatever the script says — plain text from a session-start script,
# hookSpecificOutput JSON from a tool script — in Cursor's envelope. The
# scripts stay one copy. A tool outside tool-pattern runs nothing. The answer
# is always one JSON object ("{}" for nothing to say) and the exit is always 0:
# a hook must never break the agent.
set -uo pipefail

SCRIPT="${1:-}"
PATTERN="${2:-}"
HOOK_INPUT=$(cat)
HERE="$(cd "$(dirname "$0")" && pwd)"

nothing() { printf '{}\n'; exit 0; }

# Only a hook script beside this one: a bare name ending in .sh, present here.
case "$SCRIPT" in
  ""|*/*|*..*) nothing ;;
  *.sh) [ -f "$HERE/$SCRIPT" ] || nothing ;;
  *) nothing ;;
esac
command -v python3 >/dev/null 2>&1 || nothing

# translate prints the Claude-shaped input on line 2+, after the project dir on
# line 1; it prints nothing when the tool is outside the pattern.
TRANSLATED=$(printf '%s' "$HOOK_INPUT" | python3 -c '
import json, re, sys
try:
    d = json.load(sys.stdin)
except Exception:
    d = {}
if not isinstance(d, dict):
    d = {}
pattern = sys.argv[1]
tool = d.get("tool_name") if isinstance(d.get("tool_name"), str) else ""
if pattern and not re.fullmatch(pattern, tool):
    sys.exit(0)
roots = d.get("workspace_roots")
project = roots[0] if isinstance(roots, list) and roots and isinstance(roots[0], str) else ""
events = {"sessionStart": "SessionStart", "postToolUse": "PreToolUse"}
tools = {"Shell": "Bash"}
out = dict(d)
out["hook_event_name"] = events.get(d.get("hook_event_name"), d.get("hook_event_name") or "")
if tool:
    out["tool_name"] = tools.get(tool, tool)
out["session_id"] = d.get("session_id") or d.get("conversation_id") or ""
if not out.get("cwd") and project:
    out["cwd"] = project
print(project)
print(json.dumps(out))
' "$PATTERN" 2>/dev/null)
[ -n "$TRANSLATED" ] || nothing

PROJECT=$(printf '%s\n' "$TRANSLATED" | head -1)
PAYLOAD=$(printf '%s\n' "$TRANSLATED" | tail -n +2)
if [ -z "${VAULTMIND_PROJECT_DIR:-}" ] && [ -z "${CLAUDE_PROJECT_DIR:-}" ] && [ -n "$PROJECT" ]; then
  export VAULTMIND_PROJECT_DIR="$PROJECT"
fi

OUT=$(printf '%s' "$PAYLOAD" | bash "$HERE/$SCRIPT" 2>/dev/null)

printf '%s' "$OUT" | python3 -c '
import json, sys
raw = sys.stdin.read()
text = raw.strip()
ctx = ""
if text:
    try:
        d = json.loads(text)
    except Exception:
        d = None
    if isinstance(d, dict):
        h = d.get("hookSpecificOutput")
        if isinstance(h, dict) and isinstance(h.get("additionalContext"), str):
            ctx = h["additionalContext"]
    elif d is None:
        ctx = text
print(json.dumps({"additional_context": ctx}) if ctx.strip() else "{}")
' 2>/dev/null || nothing
