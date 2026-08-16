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
# the hook works for any contributor's checkout path, not just the author's.
transcripts_subdir=$(printf '%s' "$project_dir" | sed 's|/|-|g')
transcripts_dir="$HOME/.claude/projects/$transcripts_subdir"
# Per-concern env routing: VAULTMIND_EPISODE_VAULT routes *episode writes* to
# their own vault, independent of per-turn recall and persona-load. It falls
# back to the overloaded VAULTMIND_VAULT (set by `vaultmind hooks install
# --vault`, and the simple single-var default), then to the vaultmind-identity
# convention. A dual-vault adopter can write episodes to a vault distinct from
# the recall/persona vault; a single-var setup is unchanged (issue #41.6).
vault_root="${VAULTMIND_EPISODE_VAULT:-${VAULTMIND_VAULT:-$project_dir/vaultmind-identity}}"
output_dir="$vault_root/episodes"

# Resolve vaultmind binary: prefer a project-local build (e.g.
# `<project>/bin/vaultmind` from goreleaser), then PATH-installed
# (`task install`). /tmp/vaultmind is dev-loop only (load-persona.sh
# auto-rebuild target) — NOT a fallback here. Empty `binary` falls
# back to `go run .` from the project dir as a last resort.
binary="$project_dir/bin/vaultmind"
if [[ ! -x "$binary" ]]; then
    if command -v vaultmind >/dev/null 2>&1; then
        binary=$(command -v vaultmind)
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

# --incremental: capture only the transcript delta since this session's last
# SessionEnd, not the whole transcript every time. A session that never
# closes (or is manually resumed across many technical restarts) would
# otherwise re-render into one ever-growing episode file at every SessionEnd.
if [[ -n "$binary" ]]; then
    err=$("$binary" episode capture "$transcript" --output-dir "$output_dir" --incremental 2>&1 >/dev/null) || {
        echo "capture-episode: binary run failed: $err" >&2
        exit 0
    }
else
    err=$(cd "$project_dir" && go run . episode capture "$transcript" --output-dir "$output_dir" --incremental 2>&1 >/dev/null) || {
        echo "capture-episode: go run failed: $err" >&2
        exit 0
    }
fi

# days_between prints whole days from $1 to $2 (both YYYY-MM-DD), or nothing when
# either cannot be parsed. GNU and BSD date disagree on the flag for parsing a
# date string, so both are tried: a version using only `date -j` left the gap
# permanently "unknown" on Linux, which reads as "no history" rather than "this
# platform was not handled".
days_between() {
    local a b
    a=$(date -j -f "%Y-%m-%d" "$1" +%s 2>/dev/null || date -d "$1" +%s 2>/dev/null || true)
    b=$(date -j -f "%Y-%m-%d" "$2" +%s 2>/dev/null || date -d "$2" +%s 2>/dev/null || true)
    [ -n "$a" ] && [ -n "$b" ] && echo $(( (b - a) / 86400 ))
}

# append_accumulation_record answers the one question an episode cannot: did this
# session leave anything KEPT, or only telemetry?
#
# Everything `episode capture` writes records what the session DID. A desk entry
# — a note the agent stopped to write because something landed — records what it
# UNDERSTOOD. Without this line, a run of sessions that captured perfectly and
# distilled nothing looks identical to a run that grew the vault.
#
# The desk is VAULTMIND_DESK_DIR, else <vault>/journal. When neither exists the
# section is skipped entirely rather than written with "Desk entry: NO": this
# hook must not report an absence it never looked for.
append_accumulation_record() {
    local desk_dir today ep wrote last gap days
    desk_dir="${VAULTMIND_DESK_DIR:-$vault_root/journal}"
    [ -d "$desk_dir" ] || return 0

    today=$(date +%Y-%m-%d)

    # The episode just written for this session; fall back to the newest. Silent
    # on a miss — SessionEnd must never fail over a bookkeeping addendum.
    ep=""
    if [ -n "$session_id" ]; then
        ep=$(grep -rl "session_id: $session_id" "$output_dir" 2>/dev/null | head -n1 || true)
    fi
    [ -z "$ep" ] && ep=$(ls -1t "$output_dir"/episode-*.md 2>/dev/null | head -n1 || true)
    [ -z "$ep" ] && return 0
    [ -f "$ep" ] || return 0

    grep -q "^## Accumulation" "$ep" 2>/dev/null && return 0   # idempotent

    wrote=""
    [ -n "$session_id" ] && wrote=$(grep -rl "$session_id" "$desk_dir" 2>/dev/null | head -n1 || true)
    [ -z "$wrote" ] && wrote=$(ls -1 "$desk_dir/$today"-*.md 2>/dev/null | head -n1 || true)
    last=$(ls -1 "$desk_dir" 2>/dev/null | grep -Eo '^[0-9]{4}-[0-9]{2}-[0-9]{2}' | sort | tail -1 || true)

    gap="unknown"
    if [ -n "$last" ]; then
        days=$(days_between "$last" "$today")
        [ -n "$days" ] && gap="$days days"
    fi

    {
        printf '\n## Accumulation\n\n'
        printf 'Everything above this line is telemetry — what this session DID.\n'
        printf 'This section is the only part that says whether anything was KEPT,\n'
        printf 'and it is written by the SessionEnd hook, not by `episode capture`.\n\n'
        if [ -n "$wrote" ]; then
            printf -- '- Desk entry: YES — `%s`\n' "${wrote#"$project_dir/"}"
        else
            printf -- '- Desk entry: **NO — this session left no transformation.** Its\n'
            printf '  understanding is now recoverable only by re-reading the transcript,\n'
            printf '  which is precisely the failure mode this vault exists to prevent.\n'
        fi
        printf -- '- Last desk entry: %s (gap: %s)\n' "${last:-never}" "$gap"
    } >> "$ep" 2>/dev/null || true
    return 0
}

append_accumulation_record || true

exit 0
