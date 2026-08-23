#!/usr/bin/env bash
# mesh-watch.sh — canonical wake-on-idle liveness watcher for any VaultMind agent.
#
# THE CANONICAL COPY. Shipped inside the vaultmind binary, distributed by
# `vaultmind hooks install`, drift-checked by `vaultmind hooks status`. Three
# hand-maintained ancestors preceded it (mira's poller, mira's unarmed v2,
# workhorse's live long-poller — itself "adapted from vaultmind's"); they
# diverged because nothing shipped them. This file exists so there is exactly
# one, and so a fix reaches every agent through an install instead of a copy.
#
# WHAT IT DOES. Long-polls the chat daemon's /chat_wait. When a message this
# agent should hear lands, it writes the wake context and EXITS — the harness
# re-invokes the idle session, which drains the chat and re-arms. Liveness is
# two halves, armed TOGETHER: this watcher (wake-on-event) and the
# chat_subscribe MCP tool (buffered delivery mid-turn). Neither replaces the
# other.
#
# IDENTITY. This script contains none. The first act is
#     eval "$(vaultmind identity paths)"
# and every slug, path, and URL comes from that one derivation — the same one
# `vaultmind doctor` checks against, so the watcher and its checker cannot
# disagree about where the heartbeat lives. No resolvable identity ⇒ exit 2,
# do not arm: a watcher armed as nobody writes state files a checker then
# reports on.
#
# HEARTBEAT = EVERY STREAM ALIVE. Each stream stamps its own liveness after
# every long-poll return; the supervisor writes the shared heartbeat ONLY when
# every stamp is younger than 2*(WAIT_SECS+10). An ancestor heartbeated from
# the supervisor alone, which only noticed when BOTH streams had died — one
# wedged stream heartbeated healthy forever. Present is not alive.
#
# ARMING RECORD. mesh-watch-<slug>.lastarm is stamped on every successful arm.
# doctor's rule `lastwake newer than lastarm ⇒ woke and never re-armed` is the
# one check that fires on the real 2026-08-16 death (a wake at 23:29 that no
# one followed), so the arm MUST leave a trace the wake can be compared to.
#
# SAFETY. Wakes the session to converse; takes no action itself. Kill switch:
# touch the disarm file printed by `vaultmind identity paths` (VM_MESH_DISARM).
# Bounded: exits to re-arm after MAX_WALL_SECS regardless.
set -uo pipefail   # NOT -e: a transient network hiccup must not abort the watch

# ── Identity bootstrap — the binary answers, or we refuse to arm ─────────────
PATHS_OUT="$(vaultmind identity paths 2>&1)" || {
  printf 'mesh-watch: %s\n' "$PATHS_OUT" >&2
  printf 'mesh-watch: refusing to arm without a resolved identity.\n' >&2
  exit 2
}
# Eval ONLY the VM_MESH_ assignment lines. The binary's stdout is supposed to
# be eval-clean, but "supposed to" is not a guard: workhorse's first arm had a
# JSON log line land in PATHS_OUT and bash EXECUTED it (`level:info: command
# not found`). Eval of unfiltered output is an injection surface — any stray
# stdout line with shell metacharacters runs as code here. Filtering makes the
# binary's cleanliness unnecessary to trust.
eval "$(printf '%s\n' "$PATHS_OUT" | grep '^VM_MESH_')"
if [[ -z "${VM_MESH_SLUG:-}" ]]; then
  printf 'mesh-watch: bootstrap produced no VM_MESH_ assignments — refusing to arm.\n' >&2
  exit 2
fi

WAIT_SECS="${MESH_WAIT_SECS:-28}"              # per-call long-poll window (<=30s: proxy timeout headroom)
MAX_WALL_SECS="${MESH_WATCH_MAX_WALL_SECS:-18000}"   # ~5h quiet-heartbeat ceiling
STREAM_STALE_SECS=$(( 2 * (WAIT_SECS + 10) ))  # a stream silent past two full poll cycles is wedged
BACKOFF_CAP=60                                 # transport retry ceiling (exponential from 2s)
BACKOFF_GIVEUP=10                              # consecutive transport failures before a LOUD exit

RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mesh-watch.XXXXXX")"
cleanup() {
  rm -rf "$RUN_DIR" 2>/dev/null
  local j; for j in $(jobs -p 2>/dev/null); do kill "$j" 2>/dev/null; done
  # Clear the pidfile only if it is still OURS — a successor that already took
  # over must not have its claim deleted by our late exit.
  if [[ "$(cat "$VM_MESH_PID" 2>/dev/null)" == "$$" ]]; then rm -f "$VM_MESH_PID" 2>/dev/null; fi
}
# TERM/INT included: without them a `kill` leaves the stream children orphaned,
# still long-polling forever with no parent. (Earned 2026-08-11: four stacked
# watchers left 15 orphans and woke the session three times per message.)
trap cleanup EXIT INT TERM

# ── Idempotent arming: take over any live predecessor, never stack ───────────
if [[ -f "$VM_MESH_PID" ]]; then
  prior="$(cat "$VM_MESH_PID" 2>/dev/null || true)"
  if [[ -n "${prior//[^0-9]/}" && "$prior" != "$$" ]] && kill -0 "$prior" 2>/dev/null; then
    kill "$prior" 2>/dev/null
    for _ in 1 2 3 4 5 6 7 8 9 10; do kill -0 "$prior" 2>/dev/null || break; sleep 0.2; done
    kill -9 "$prior" 2>/dev/null || true
  fi
fi
echo $$ > "$VM_MESH_PID" 2>/dev/null || true

# ── Detector: stdin = /chat_wait JSON; prints "from|ts" of the newest message
#    this agent should hear (nothing if none). ALL identity arrives via env —
#    a quoted heredoc cannot interpolate, and trying is how an ancestor spelled
#    its own slug four times. Exit: 0 = clean parse, anything else = escalate.
read -r -d '' DETECT <<'PY' || true
import sys, json, os

SELF = os.environ["VM_WAKE_SELF"]
OPERATOR = os.environ.get("VM_WAKE_OPERATOR", "")
LISTEN = os.environ.get("VM_WAKE_LISTEN_FILE", "")
SCOPE = os.environ.get("VM_WAKE_SCOPE", "all")
ROOM_SINCE = int(os.environ.get("VM_WAKE_ROOM_SINCE", "0") or "0")
DM_SINCE = int(os.environ.get("VM_WAKE_DM_SINCE", "0") or "0")
WAKE_FROM = [x for x in os.environ.get("VM_WAKE_FROM", "").split(",") if x]
KEYWORDS = [x for x in os.environ.get("VM_WAKE_KEYWORDS", "").split(",") if x]

def _listen_state():
    try:
        with open(LISTEN) as f:
            return json.load(f)
    except Exception:
        return {"mode": "all", "hear": [], "mute": [], "hard": False}

def hear(frm, s):
    # The `listen` control's global filter. The operator is ALWAYS heard unless
    # hard mode — you can mute agents, never accidentally mute your human.
    if frm == SELF:
        return False
    if OPERATOR and frm == OPERATOR and not s.get("hard", False):
        return True
    m = s.get("mode", "all")
    if m == "only":
        return frm in s.get("hear", [])
    if m == "except":
        return frm not in s.get("mute", [])
    return True

def relevant(m):
    # Per-type local floors: the combined room+DM call shares ONE wire `since`
    # (the MIN of both baselines, so neither stream misses), which means the
    # wire can replay an already-seen message from the OTHER stream. Each
    # message type is re-filtered against its own true baseline here.
    ts = m.get("ts", 0) or 0
    # A DM is anything not stamped with a room, or stamped as addressed to me —
    # two daemon conventions, either sufficient (belt and braces: an ancestor
    # keyed on only one and would misfire if the daemon ever stamped the other).
    is_dm = (not m.get("room")) or (m.get("to_agent") == SELF)
    if is_dm:
        return ts > DM_SINCE
    if ts <= ROOM_SINCE:
        return False
    if SCOPE != "filtered":
        return True
    frm = m.get("from_agent", "")
    if frm in WAKE_FROM:
        return True
    body = (m.get("body") or "").lower()
    return any(k.lower() in body for k in KEYWORDS)

try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(3)
s = _listen_state()
hits = [m for m in d.get("messages", []) if hear(m.get("from_agent", ""), s) and relevant(m)]
if hits:
    m = hits[-1]
    print(str(m.get("from_agent", "?")) + "|" + str(m.get("ts", "")))
PY

# ── Arm-time detector self-test — "armed" must mean the detector WORKS ───────
# A missing interpreter exits 127, a truncated heredoc is an empty program that
# exits 0 printing nothing, forever. Both used to read as "no message". Feed a
# fixture whose answer is known; anything but that answer refuses to arm.
selftest="$(printf '{"messages":[{"from_agent":"agent:selftest","ts":42,"room":"mesh"}]}' \
  | VM_WAKE_SELF="$VM_MESH_SELF" VM_WAKE_OPERATOR="" VM_WAKE_SCOPE="all" \
    VM_WAKE_ROOM_SINCE=0 VM_WAKE_DM_SINCE=0 VM_WAKE_LISTEN_FILE="$VM_MESH_LISTEN" \
    VM_WAKE_FROM="" VM_WAKE_KEYWORDS="" python3 -c "$DETECT" 2>&1)" || {
  printf 'mesh-watch: detector self-test FAILED (%s) — refusing to arm with a broken detector.\n' "$selftest" >&2
  exit 2
}
if [[ "$selftest" != "agent:selftest|42" ]]; then
  printf 'mesh-watch: detector self-test returned %q, expected "agent:selftest|42" — refusing to arm.\n' "$selftest" >&2
  exit 2
fi

# ── Room list: policy from the shared listen file, not from code ─────────────
# agents.<slug>.rooms in mesh-listen.json wins; absent ⇒ mesh + keeper-of-the-
# realm at scope "all" (both live deployments' union). "filtered" scope wakes
# only on wake_from principals (default: the operator) or keyword hits
# (default: the agent's own slug). One line per room: name|scope|from|keywords
ROOMS="$(VM_ROOMS_SLUG="$VM_MESH_SLUG" VM_ROOMS_OPERATOR="${VM_MESH_OPERATOR:-}" \
         VM_ROOMS_LISTEN="$VM_MESH_LISTEN" python3 - <<'PY'
import json, os
slug = os.environ["VM_ROOMS_SLUG"]
op = os.environ.get("VM_ROOMS_OPERATOR", "")
try:
    with open(os.environ["VM_ROOMS_LISTEN"]) as f:
        cfg = json.load(f)
except Exception:
    cfg = {}
rooms = (cfg.get("agents", {}).get(slug, {}) or {}).get("rooms") or {
    "mesh": {"scope": "all"},
    "keeper-of-the-realm": {"scope": "all"},
}
for name, rc in rooms.items():
    scope = rc.get("scope", "all")
    frm = ",".join(rc.get("wake_from", [op] if op else []))
    kw = ",".join(rc.get("keywords", [slug]))
    print(f"{name}|{scope}|{frm}|{kw}")
PY
)" || { printf 'mesh-watch: room resolution failed — refusing to arm.\n' >&2; exit 2; }
[[ -z "$ROOMS" ]] && { printf 'mesh-watch: no rooms resolved — refusing to arm.\n' >&2; exit 2; }

# ── Baselines: newest ts per stream at arm time. Unreachable ≠ empty ─────────
# An ancestor returned 0 on transport failure, so a daemon blip at arm time
# fabricated an epoch baseline, replayed all history, and wake-stormed. A
# watcher that cannot see the store has not armed.
newest_ts() {
  local out
  out="$(curl -fsS -m 10 "$1" 2>/dev/null)" || { echo -1; return; }
  printf '%s' "$out" | python3 -c 'import sys,json
d=json.load(sys.stdin); m=d.get("messages",[]); print(m[-1].get("ts",0) if m else 0)' 2>/dev/null || echo -1
}

dm_baseline="$(newest_ts "${VM_MESH_DAEMON}/chat_read?to_agent=${VM_MESH_SELF}")"
if [[ "$dm_baseline" -lt 0 ]]; then
  printf 'mesh-watch: cannot establish a baseline — daemon unreachable at %s. Refusing to arm against a fabricated zero.\n' "$VM_MESH_DAEMON" >&2
  exit 2
fi

start_ts=$(date +%s)

# wait_loop <idx> <room> <scope> <from> <keywords> <first(1|0)>
# The FIRST room's call folds in the DM mailbox (the daemon wakes on either
# from one call), so N rooms cost N connections, not N+1.
wait_loop() {
  local idx="$1" room="$2" scope="$3" wfrom="$4" kws="$5" first="$6"
  local sig="${RUN_DIR}/s${idx}.sig" alive="${RUN_DIR}/s${idx}.alive"
  local backoff=2 failures=0 out hit rc newest

  local room_baseline
  room_baseline="$(newest_ts "${VM_MESH_DAEMON}/chat_read?room=${room}")"
  if [[ "$room_baseline" -lt 0 ]]; then
    echo "WATCHER ERROR: could not baseline room ${room} (daemon unreachable at arm) — not armed for it." > "$sig"
    return 0
  fi

  local query="/chat_wait?room=${room}" since="$room_baseline" dm_floor=0
  if [[ "$first" == "1" ]]; then
    query="${query}&to_agent=${VM_MESH_SELF}"
    dm_floor="$dm_baseline"
    # Shared wire `since` must be the MIN of both baselines so neither stream
    # misses; the per-type floors in DETECT filter the resulting replays.
    if [[ "$dm_baseline" -lt "$since" ]]; then since="$dm_baseline"; fi
  fi

  while true; do
    date +%s > "$alive" 2>/dev/null || true
    [[ -f "$VM_MESH_DISARM" ]] && return 0
    (( $(date +%s) - start_ts > MAX_WALL_SECS )) && return 0

    out="$(curl -fsS -m $((WAIT_SECS + 10)) "${VM_MESH_DAEMON}${query}&since=${since}&timeout_secs=${WAIT_SECS}" 2>/dev/null)"
    if [[ -z "$out" ]]; then
      failures=$((failures + 1))
      if (( failures >= BACKOFF_GIVEUP )); then
        # An outage should wake the session ONCE, loudly — not sit dark for
        # five hours and then print "no new message".
        echo "WATCHER ERROR: ${room} transport down after ${failures} consecutive failures — daemon ${VM_MESH_DAEMON} unreachable. Fix the connection, then re-arm." > "$sig"
        return 0
      fi
      sleep "$backoff"
      backoff=$(( backoff * 2 )); (( backoff > BACKOFF_CAP )) && backoff=$BACKOFF_CAP
      continue
    fi
    failures=0; backoff=2

    hit="$(printf '%s' "$out" | \
      VM_WAKE_SELF="$VM_MESH_SELF" VM_WAKE_OPERATOR="${VM_MESH_OPERATOR:-}" \
      VM_WAKE_SCOPE="$scope" VM_WAKE_ROOM_SINCE="$room_baseline" VM_WAKE_DM_SINCE="$dm_floor" \
      VM_WAKE_LISTEN_FILE="$VM_MESH_LISTEN" VM_WAKE_FROM="$wfrom" VM_WAKE_KEYWORDS="$kws" \
      python3 -c "$DETECT")"; rc=$?
    if (( rc != 0 )); then
      # ANY nonzero rc escalates, with the rc in the text. An ancestor tested
      # rc==3 only, so a missing interpreter (127), OOM (137) or a traceback
      # (1) all read as "no message" — a one-integer-wide guard.
      echo "WATCHER ERROR: ${room} detector exited rc=${rc} (waking to fix — NOT silently treating as no-message)." > "$sig"
      return 0
    fi
    if [[ -n "$hit" ]]; then
      local ts="${hit##*|}" recover="${hit##*|}"
      [[ "$ts" =~ ^[0-9]+$ ]] && recover="$(( ts - 1 ))"
      printf '%s' "$hit" > "$VM_MESH_LASTWAKE" 2>/dev/null || true
      # The empty-drain disambiguation must travel IN the wake line: only this
      # watcher has PROVEN a message exists, so an empty chat_drain after this
      # wake means the subscription is dead, not that nothing arrived.
      # (Observed live 2026-08-12: real wake, drain [], subscriber_count 0.)
      echo "WAKE: new ${room} message from ${hit%%|*} (ts ${ts}). Drain the chat (rooms + DMs), respond, then RE-ARM mesh-watch. IF THE DRAIN COMES BACK EMPTY do NOT conclude there is nothing — this message provably exists; re-subscribe and read ${room} with since=${recover} to recover it." > "$sig"
      return 0
    fi

    newest="$(printf '%s' "$out" | python3 -c 'import sys,json
d=json.load(sys.stdin); m=d.get("messages",[]); print(m[-1].get("ts",0) if m else 0)' 2>/dev/null || echo 0)"
    [[ -n "$newest" && "$newest" -gt "$since" ]] && since="$newest"
  done
}

# ── Fork one stream per room; supervisor owns the heartbeat ──────────────────
idx=0 first=1 PIDS=()
while IFS='|' read -r room scope wfrom kws; do
  [[ -z "$room" ]] && continue
  wait_loop "$idx" "$room" "$scope" "$wfrom" "$kws" "$first" &
  PIDS+=($!)
  idx=$((idx + 1)); first=0
done <<< "$ROOMS"
NSTREAMS=$idx

date +%s > "$VM_MESH_LASTARM" 2>/dev/null || true   # the arm leaves its trace

hb_failures=0
while true; do
  # Exit conditions first: any signal, all children dead, or wall-clock.
  for ((i=0; i<NSTREAMS; i++)); do [[ -f "${RUN_DIR}/s${i}.sig" ]] && break 2; done
  alive_children=0
  for p in "${PIDS[@]}"; do kill -0 "$p" 2>/dev/null && alive_children=1; done
  (( alive_children == 0 )) && break
  (( $(date +%s) - start_ts > MAX_WALL_SECS )) && break

  # Heartbeat ONLY when every stream proved life within the window.
  now=$(date +%s); all_alive=1
  for ((i=0; i<NSTREAMS; i++)); do
    stamp="$(cat "${RUN_DIR}/s${i}.alive" 2>/dev/null || echo 0)"
    (( now - stamp > STREAM_STALE_SECS )) && { all_alive=0; break; }
  done
  if (( all_alive )); then
    if date +%s > "$VM_MESH_HEARTBEAT" 2>/dev/null; then
      hb_failures=0
    else
      hb_failures=$((hb_failures + 1))
      if (( hb_failures >= 3 )); then
        # A watcher that cannot write its heartbeat is invisible to doctor
        # while running — permanently present-but-dead. Say so and stop.
        echo "WATCHER ERROR: cannot write heartbeat at ${VM_MESH_HEARTBEAT} (3 consecutive failures — disk full or permissions?). Watcher exiting; fix and re-arm."
        exit 0
      fi
    fi
  fi
  sleep 2
done

for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null; done
for p in "${PIDS[@]}"; do wait "$p" 2>/dev/null; done

for ((i=0; i<NSTREAMS; i++)); do
  [[ -f "${RUN_DIR}/s${i}.sig" ]] && { cat "${RUN_DIR}/s${i}.sig"; exit 0; }
done

if [[ -f "$VM_MESH_DISARM" ]]; then echo "DISARMED (sentinel present) — not re-arming."; exit 0; fi
echo "RE-ARM: no relevant message within ~${MAX_WALL_SECS}s (quiet heartbeat) — re-launch mesh-watch.sh."
exit 0
