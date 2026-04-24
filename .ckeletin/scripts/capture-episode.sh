#!/usr/bin/env bash
# capture-episode.sh
#
# Called from the Claude Code SessionEnd / Stop hook. Parses the current
# session's JSONL transcript into a markdown "episode" file under
# vaultmind-identity/episodes/. Episodic substrate v0 — no distillation,
# no indexing, just durable per-session capture.
#
# Reads the hook JSON payload from stdin (Claude Code convention) to get
# session_id. Falls back to the most recently modified transcript in the
# project's transcripts directory if the payload is absent or unreadable.
#
# Exits 0 on success or graceful-degradation paths: a failed capture must
# never block the user's session end. Errors go to stderr for debugging.

set -eu

project_dir="${CLAUDE_PROJECT_DIR:-$(pwd)}"

# Claude Code encodes the absolute project directory path into the transcripts
# subdirectory name by replacing "/" with "-". Derive instead of hardcoding so
# the hook works for any contributor's checkout path, not just Peiman's.
transcripts_subdir=$(printf '%s' "$project_dir" | sed 's|/|-|g')
transcripts_dir="$HOME/.claude/projects/$transcripts_subdir"
output_dir="$project_dir/vaultmind-identity/episodes"
binary="$project_dir/bin/vaultmind"

# Prefer the project-local binary; fall back to /tmp/vaultmind (dev
# convenience), then to `go run .` as a last resort.
if [[ ! -x "$binary" ]]; then
    if [[ -x /tmp/vaultmind ]]; then
        binary="/tmp/vaultmind"
    else
        binary=""
    fi
fi

# Read the hook payload (JSON) from stdin if available — non-blocking.
payload=""
if [[ ! -t 0 ]]; then
    payload=$(cat || true)
fi

session_id=""
if [[ -n "$payload" ]]; then
    if command -v jq >/dev/null 2>&1; then
        session_id=$(printf '%s' "$payload" | jq -r '.session_id // empty' 2>/dev/null || true)
    else
        echo "capture-episode: jq not found; falling back to most-recent transcript (risks capturing the wrong session under concurrent sessions in the same repo)" >&2
    fi
fi

transcript=""
if [[ -n "$session_id" && -f "$transcripts_dir/$session_id.jsonl" ]]; then
    transcript="$transcripts_dir/$session_id.jsonl"
elif [[ -d "$transcripts_dir" ]]; then
    # Fallback: most recently modified .jsonl in this project.
    transcript=$(ls -1t "$transcripts_dir"/*.jsonl 2>/dev/null | head -n1 || true)
fi

if [[ -z "$transcript" ]]; then
    echo "capture-episode: no transcript found (session=$session_id)" >&2
    exit 0
fi

mkdir -p "$output_dir"

if [[ -n "$binary" ]]; then
    "$binary" episode capture "$transcript" --output-dir "$output_dir" >/dev/null 2>&1 || {
        echo "capture-episode: binary run failed" >&2
        exit 0
    }
else
    (cd "$project_dir" && go run . episode capture "$transcript" --output-dir "$output_dir" >/dev/null 2>&1) || {
        echo "capture-episode: go run failed" >&2
        exit 0
    }
fi

exit 0
