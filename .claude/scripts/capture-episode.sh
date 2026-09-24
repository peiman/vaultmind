#!/usr/bin/env bash
# capture-episode.sh
#
# Called from the SessionEnd hook of Claude Code or Codex CLI. Parses the
# current session's JSONL transcript into a markdown "episode" file under
# vaultmind-identity/episodes/. Episodic substrate v0 — no distillation,
# no indexing, just durable per-session capture.
#
# WHICH TRANSCRIPT. The hook payload on stdin names it: transcript_path (sent
# by Claude Code and by Codex), else session_id under Claude Code's transcript
# directory. A session the payload NAMES but whose file cannot be found is
# captured as nothing, with a message — never replaced by the newest
# transcript, which is some other session (always, under Codex, whose
# transcripts live elsewhere; sometimes, with two Claude sessions in one repo).
# Only with no payload at all is the newest transcript the fallback.
#
# Exits 0 on success or graceful-degradation paths: a failed capture must
# never block the user's session end. Errors go to stderr for debugging.

set -eu

project_dir="${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$(pwd)}}"

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
transcript_path=""
if [[ -n "$payload" ]]; then
    if command -v jq >/dev/null 2>&1; then
        session_id=$(printf '%s' "$payload" | jq -r '.session_id // empty' 2>/dev/null || true)
        transcript_path=$(printf '%s' "$payload" | jq -r '.transcript_path // empty' 2>/dev/null || true)
    else
        echo "capture-episode: jq not found; falling back to most-recent transcript (risks capturing the wrong session under concurrent sessions in the same repo)" >&2
    fi
fi

# One line per run in ~/.vaultmind/capture/capture.log (the scripts' log
# convention): SessionEnd output is shown nowhere, under Codex or Claude Code,
# so without this a failed capture and a hook that never ran look identical —
# no episode. Found on the first live Codex test, 2026-09-24. Bounded, and
# never allowed to fail the hook.
capture_log="${VAULTMIND_CAPTURE_LOG:-$HOME/.vaultmind/capture/capture.log}"
record_capture() {
    {
        mkdir -p "$(dirname "$capture_log")" &&
            printf '%s\t%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$project_dir" "${session_id:--}" "$1" >>"$capture_log" &&
            if [ "$(wc -l <"$capture_log")" -gt 1000 ]; then
                tail -n 500 "$capture_log" >"$capture_log.tmp" && mv -f "$capture_log.tmp" "$capture_log"
            fi
    } 2>/dev/null || true
}

transcript=""
if [[ -n "$transcript_path" ]]; then
    # The payload named the file. Use it or nothing — a named file that is
    # missing is not permission to capture a different one.
    if [[ -f "$transcript_path" ]]; then
        transcript="$transcript_path"
    else
        echo "capture-episode: the session's transcript is not there: $transcript_path — nothing captured" >&2
        record_capture "not captured: transcript not found at $transcript_path"
        exit 0
    fi
elif [[ -n "$session_id" ]]; then
    if [[ -f "$transcripts_dir/$session_id.jsonl" ]]; then
        transcript="$transcripts_dir/$session_id.jsonl"
    else
        echo "capture-episode: no transcript for session $session_id in $transcripts_dir — nothing captured (not substituting another session's)" >&2
        record_capture "not captured: no transcript for this session in $transcripts_dir"
        exit 0
    fi
elif [[ -d "$transcripts_dir" ]]; then
    # No payload at all: no session to be wrong about, so the newest transcript
    # in this project is the only available guess.
    transcript=$(ls -1t "$transcripts_dir"/*.jsonl 2>/dev/null | head -n1 || true)
fi

if [[ -z "$transcript" ]]; then
    echo "capture-episode: no transcript found (session=$session_id)" >&2
    record_capture "not captured: no transcript found"
    exit 0
fi

mkdir -p "$output_dir"

# --incremental: capture only the transcript delta since this session's last
# SessionEnd, not the whole transcript every time. A session that never
# closes (or is manually resumed across many technical restarts) would
# otherwise re-render into one ever-growing episode file at every SessionEnd.
# stdout is the episode path written, or "(nothing new …)" when the session
# had nothing to capture (e.g. opened and quit); stderr is kept for failures.
errf=$(mktemp 2>/dev/null || echo "/tmp/capture-episode.$$")
if [[ -n "$binary" ]]; then
    out=$("$binary" episode capture "$transcript" --output-dir "$output_dir" --incremental 2>"$errf") || {
        err=$(cat "$errf" 2>/dev/null); rm -f "$errf"
        echo "capture-episode: binary run failed: $err" >&2
        record_capture "failed: $(printf '%s' "$err" | tr '\t\n' '  ' | cut -c1-200)"
        exit 0
    }
else
    out=$(cd "$project_dir" && go run . episode capture "$transcript" --output-dir "$output_dir" --incremental 2>"$errf") || {
        err=$(cat "$errf" 2>/dev/null); rm -f "$errf"
        echo "capture-episode: go run failed: $err" >&2
        record_capture "failed: $(printf '%s' "$err" | tr '\t\n' '  ' | cut -c1-200)"
        exit 0
    }
fi
rm -f "$errf"
episode_written=$(printf '%s\n' "$out" | grep '\.md$' | tail -n 1)
if [[ -n "$episode_written" ]]; then
    record_capture "captured $episode_written"
else
    record_capture "nothing new to capture"
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
