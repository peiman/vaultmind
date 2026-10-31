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
# `identity paths` falls back to the CURRENT DIRECTORY for the project; a watcher
# started while the shell sat in a subfolder resolved no identity and refused to
# arm (2026-09-24). Pin the project this script belongs to, unless the harness
# already set it (chat-mcp does). Two levels up from .claude/scripts or
# .vaultmind/scripts is the project.
export AGENT_CHAT_PROJECT_PATH="${AGENT_CHAT_PROJECT_PATH:-${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}}}"
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

# ── Registry countdown — carried into every wake, because a freshness warning
# only in `doctor` is a warning nobody reads. On 2026-09-06 the trust registry
# aged past the hub's max_staleness and EVERY signed send was refused for ~21
# hours while reads, long-poll and this watcher all stayed green: the mute is
# invisible from inside, so the number has to ride the message the agent
# provably reads at the moment it is about to use chat.
#
# Empty when no bound is declared in agents.yaml (registry_max_staleness_secs).
# An undeclared bound yields NO countdown rather than a guessed one — a
# confident wrong date is worse than none, because someone plans around it.
registry_countdown() {
  # READ AT WAKE, NOT AT ARM. The value used to come only from the bootstrap
  # eval when the watcher was armed, and was printed at wake — up to five hours
  # later. On 2026-09-23 a wake line said "fresh for 12 more day(s)" hours after
  # a re-sign had put a 30-day registry live. So re-read it now. Only the one
  # assignment is taken and it is never eval'd (the bootstrap's injection lesson).
  # If the re-read fails the arm-time number is still the best information, but
  # it is LABELLED as old: an unlabelled stale number is the defect itself.
  #
  # FETCH FIRST. Member machines had no local copy of the registry (dabir,
  # 2026-09-22), so there was nothing to count down from, and a re-sign only
  # reached a machine if someone copied the file. `identity fetch-registry`
  # takes the hub's copy, verified against the pinned root and never older than
  # the local one. A failure is SAID in the wake line — a silent one is how a
  # machine keeps counting from a registry the hub has replaced — and never
  # blocks the wake. Only the command's own "Error:" line is shown, not its logs.
  if ! _vm_fetch="$(vaultmind identity fetch-registry 2>&1)"; then
    _vm_fetch_err="$(printf '%s\n' "$_vm_fetch" | grep '^Error: ' | tail -n 1)"
    _vm_fetch_err="${_vm_fetch_err#Error: }"
    printf ' [registry fetch failed: %s]' "${_vm_fetch_err:-(no output)}"
  fi
  _vm_days_note=""
  if _vm_now="$(vaultmind identity paths 2>/dev/null)"; then
    _vm_now="$(printf '%s\n' "$_vm_now" | grep '^VM_MESH_REGISTRY_DAYS_LEFT=' | head -n 1)"
    _vm_now="${_vm_now#VM_MESH_REGISTRY_DAYS_LEFT=}"
    _vm_now="${_vm_now#\'}"
    VM_MESH_REGISTRY_DAYS_LEFT="${_vm_now%\'}"
  elif [ -n "${VM_MESH_REGISTRY_DAYS_LEFT:-}" ]; then
    _vm_days_note=" (as of arm time — could not re-read it)"
  fi
  [ -n "${VM_MESH_REGISTRY_DAYS_LEFT:-}" ] || return 0
  # VALIDATE FIRST. Both numeric comparisons below exit 2 on a non-numeric
  # value, so an unvalidated version fell through to the else-branch and
  # rendered "[registry fresh for garbage more day(s)]" — corruption reported
  # as health, which is the exact class this countdown exists to end.
  # Strip ONE leading sign, then demand digits only. The previous pattern
  # admitted interior and trailing '-' ("5-3", "0-"), and a bignum bash cannot
  # compare — all of which fell through to the reassuring branch.
  # Width-bounded as well as digits-only: a value can be all digits and still
  # be uncomparable to bash ("-999999999999999999999"), and an uncomparable
  # value used to land in the reassuring branch. No real countdown needs more
  # than 9 digits.
  _vm_days_abs="${VM_MESH_REGISTRY_DAYS_LEFT#-}"
  case "$_vm_days_abs" in
    ''|*[!0-9]*)
      printf ' [registry countdown UNAVAILABLE: %s — cannot tell you when sends start failing]' \
        "$VM_MESH_REGISTRY_DAYS_LEFT"
      return 0 ;;
  esac
  if [ "${#_vm_days_abs}" -gt 9 ]; then
    printf ' [registry countdown UNAVAILABLE: %s — cannot tell you when sends start failing]' \
      "$VM_MESH_REGISTRY_DAYS_LEFT"
    return 0
  fi
  if [ "$VM_MESH_REGISTRY_DAYS_LEFT" -lt 0 ]; then
    # Strictly less than zero. Days-left FLOORS, so 0 means up to 24 hours of
    # send-life remaining — announcing "SENDS REFUSED" on that day is a false
    # present-tense alarm whose remedy is an offline-root signing ceremony.
    # The sign is carried by the word "past", not printed twice.
    printf ' REGISTRY STALE (%s day(s) past the hub bound) — your signed SENDS are being REFUSED while reads still work; re-sign and redeploy the registry.' \
      "${VM_MESH_REGISTRY_DAYS_LEFT#-}"
  elif [ "$VM_MESH_REGISTRY_DAYS_LEFT" -le 7 ]; then
    printf ' REGISTRY EXPIRES IN %s DAY(S) — re-sign and redeploy before sends start failing.' \
      "$VM_MESH_REGISTRY_DAYS_LEFT"
  else
    printf ' [registry fresh for %s more day(s)]' "$VM_MESH_REGISTRY_DAYS_LEFT"
  fi
  printf '%s' "$_vm_days_note"
}

WAIT_SECS="${MESH_WAIT_SECS:-28}"              # per-call long-poll window (<=30s: proxy timeout headroom)
MAX_WALL_SECS="${MESH_WATCH_MAX_WALL_SECS:-18000}"   # ~5h quiet-heartbeat ceiling
STREAM_STALE_SECS=$(( 2 * (WAIT_SECS + 10) ))  # a stream silent past two full poll cycles is wedged
BACKOFF_CAP=60                                 # transport retry ceiling (exponential from 2s)
SWEEP_SECS="${MESH_SWEEP_SECS:-15}"            # overlap-read cadence (same-second stragglers, #350)
# A hub outage is ridden out, then reported once, LOUDLY (#371). A fixed count
# of failed reads gave up after ~150 s, so every short hub blip left the agent
# deaf until a manual re-arm (four times on 2026-10-29, once for 32 hours).
OUTAGE_GIVEUP_SECS="${MESH_OUTAGE_GIVEUP_SECS:-600}"

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
#
# A stop must END the watcher, and say why (#362, #363). The handler used to be
# cleanup alone: it killed the streams and the script RESUMED, so the main loop
# saw every stream dead and printed "WATCHER ERROR … re-arm", exit 3. The
# session re-armed, which took over the newer watcher, which said the same:
# two watchers taking the pidfile from each other. A watcher replaced by a
# newer one now says so and does not ask for a re-arm.
on_stop() {
  local now
  now="$(cat "$VM_MESH_PID" 2>/dev/null || true)"
  if [[ -n "${now//[^0-9]/}" && "$now" != "$$" ]] && kill -0 "$now" 2>/dev/null; then
    echo "REPLACED: a newer mesh-watch (pid ${now}) took over and is watching. Nothing to do; do not re-arm."
  else
    echo "STOPPED: mesh-watch was stopped and no other watcher holds the pidfile. Re-arm if you still want to be woken."
  fi
  exit 0
}
trap cleanup EXIT
trap on_stop INT TERM

# ── Idempotent arming: take over any live predecessor, never stack ───────────
# The claim comes first, so the predecessor's stop handler can name its
# successor. The predecessor gets 5 s to clean up its streams before SIGKILL,
# which would orphan them.
prior="$(cat "$VM_MESH_PID" 2>/dev/null || true)"
echo $$ > "$VM_MESH_PID" 2>/dev/null || true
if [[ -n "${prior//[^0-9]/}" && "$prior" != "$$" ]] && kill -0 "$prior" 2>/dev/null; then
  kill "$prior" 2>/dev/null
  for _ in $(seq 1 25); do kill -0 "$prior" 2>/dev/null || break; sleep 0.2; done
  kill -9 "$prior" 2>/dev/null || true
fi

# ── Detector: stdin = /chat_wait JSON; prints "from|ts" of the newest message
#    this agent should hear (nothing if none). ALL identity arrives via env —
#    a quoted heredoc cannot interpolate, and trying is how an ancestor spelled
#    its own slug four times. Exit: 0 = clean parse, anything else = escalate.
read -r -d '' DETECT <<'PY' || true
import sys, json, os, hashlib, tempfile

SELF = os.environ["VM_WAKE_SELF"]
OPERATOR = os.environ.get("VM_WAKE_OPERATOR", "")
LISTEN = os.environ.get("VM_WAKE_LISTEN_FILE", "")
SCOPE = os.environ.get("VM_WAKE_SCOPE", "all")
CURSOR_FILE = os.environ.get("VM_WAKE_CURSOR_FILE", "")
if CURSOR_FILE:
    with open(CURSOR_FILE) as f:
        cursors = json.load(f)
else:
    cursors = {kind: json.loads(os.environ.get("VM_WAKE_" + kind.upper() + "_CURSOR",
                                             '{"ts":0,"ids":[]}'))
               for kind in ("room", "dm")}
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

def stream(m):
    # A DM is anything not stamped with a room, or stamped as addressed to me —
    # two daemon conventions, either sufficient (belt and braces: an ancestor
    # keyed on only one and would misfire if the daemon ever stamped the other).
    return "dm" if (not m.get("room")) or (m.get("to_agent") == SELF) else "room"

def relevant(m):
    if stream(m) == "dm":
        return True
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
hits = []
# Filter the entire batch against one snapshot: receive order is not ts order.
snapshot = {kind: {"ts": c["ts"], "ids": set(c["ids"])}
            for kind, c in cursors.items()}
observed = {kind: set() for kind in cursors}
def message_id(m):
    return m.get("id") or "sha256:" + hashlib.sha256(json.dumps(
        [m.get("from_agent"), m.get("ts"), m.get("body")],
        separators=(",", ":")).encode()).hexdigest()

for m in d.get("messages", []):
    kind = stream(m)
    cursor = snapshot.get(kind)
    if cursor is None:
        continue
    ts, mid = m.get("ts", 0) or 0, message_id(m)
    if ts < cursor["ts"] or (ts == cursor["ts"] and mid in cursor["ids"]):
        continue
    if (ts, mid) in observed[kind]:
        continue
    observed[kind].add((ts, mid))
    if hear(m.get("from_agent", ""), s) and relevant(m):
        hits.append(m)
# Retain only identities at the high-water second, including filtered messages.
for kind, seen in observed.items():
    old = snapshot[kind]
    newest = max([old["ts"]] + [ts for ts, _ in seen])
    ids = old["ids"] if newest == old["ts"] else set()
    cursors[kind] = {"ts": newest,
                     "ids": sorted(ids | {mid for ts, mid in seen if ts == newest})}
if CURSOR_FILE:
    # Readers see either complete generation, even if the process is stopped.
    name = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", dir=os.path.dirname(CURSOR_FILE),
                                         delete=False) as f:
            name = f.name
            json.dump(cursors, f)
        os.replace(name, CURSOR_FILE)
    finally:
        if name and os.path.exists(name):
            os.unlink(name)
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
    VM_WAKE_LISTEN_FILE="$VM_MESH_LISTEN" \
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

# ── Baselines: newest ts AND IDs at that second. Unreachable ≠ empty ─────────
# An ancestor returned 0 on transport failure, so a daemon blip at arm time
# fabricated an epoch baseline, replayed all history, and wake-stormed. A
# watcher that cannot see the store has not armed.
newest_cursor() {
  local out err="${2:-${RUN_DIR:+$RUN_DIR/baseline.err}}"
  out="$(curl -fsS -m 10 "$1" 2>"${err:-/dev/null}")" || return 1
  printf '%s' "$out" | python3 -c 'import sys,json,hashlib
def message_id(m):
    return m.get("id") or "sha256:" + hashlib.sha256(json.dumps(
        [m.get("from_agent"), m.get("ts"), m.get("body")],
        separators=(",", ":")).encode()).hexdigest()
d=json.load(sys.stdin); messages=d.get("messages",[])
ts=max((m.get("ts",0) or 0 for m in messages), default=0)
print(json.dumps({"ts":ts,"ids":sorted({message_id(m) for m in messages if (m.get("ts",0) or 0)==ts})}))' 2>/dev/null
}

# A baseline that cannot be read is retried through an outage, never faked.
# Prints the cursor, or returns 1 after OUTAGE_GIVEUP_SECS with the last
# curl error in $2 (each caller its own file: streams baseline in parallel).
baseline_retry() {
  local t0 b=2
  t0=$(date +%s)
  until newest_cursor "$1" "${2:-}"; do
    (( $(date +%s) - t0 >= OUTAGE_GIVEUP_SECS )) && return 1
    sleep "$b"
    b=$(( b * 2 )); (( b > BACKOFF_CAP )) && b=$BACKOFF_CAP
  done
  # Explicit: an until loop's status is its body's last command, and the cap
  # test above is false (1) whenever the backoff is under the cap.
  return 0
}

if ! dm_baseline="$(baseline_retry "${VM_MESH_DAEMON}/chat_read?to_agent=${VM_MESH_SELF}")"; then
  printf 'mesh-watch: cannot establish a baseline — daemon unreachable at %s for %ss (last error: %s). Refusing to arm against a fabricated zero.\n' \
    "$VM_MESH_DAEMON" "$OUTAGE_GIVEUP_SECS" "$(tail -n1 "${RUN_DIR}/baseline.err" 2>/dev/null)" >&2
  exit 2
fi

start_ts=$(date +%s)

# wait_loop <idx> <room> <scope> <from> <keywords> <first(1|0)>
# The FIRST room's call folds in the DM mailbox (the daemon wakes on either
# from one call), so N rooms cost N connections, not N+1.
wait_loop() {
  local idx="$1" room="$2" scope="$3" wfrom="$4" kws="$5" first="$6"
  local sig="${RUN_DIR}/s${idx}.sig" alive="${RUN_DIR}/s${idx}.alive"
  local backoff=2 out hit rc since
  local cursor="${RUN_DIR}/s${idx}.cursor"

  local room_baseline
  if ! room_baseline="$(baseline_retry "${VM_MESH_DAEMON}/chat_read?room=${room}" "${RUN_DIR}/s${idx}.baseline.err")"; then
    echo "WATCHER ERROR: could not baseline room ${room} — daemon unreachable for ${OUTAGE_GIVEUP_SECS}s at arm (last error: $(tail -n1 "${RUN_DIR}/s${idx}.baseline.err" 2>/dev/null)). Not armed for it." > "$sig"
    return 0
  fi

  local query="/chat_wait?room=${room}"
  if [[ "$first" == "1" ]]; then
    query="${query}&to_agent=${VM_MESH_SELF}"
    printf '{"room":%s,"dm":%s}' "$room_baseline" "$dm_baseline" > "${cursor}.tmp"
  else
    printf '{"room":%s}' "$room_baseline" > "${cursor}.tmp"
  fi

  # An empty mailbox has no historical second to overlap. Seed it from the
  # newest arm cursor (or arm time if both stores are empty), never epoch - 1.
  python3 - "${cursor}.tmp" "$start_ts" <<'PYINIT'
import json, sys
with open(sys.argv[1]) as f:
    cursors = json.load(f)
newest = max(c["ts"] for c in cursors.values()) or int(sys.argv[2])
for c in cursors.values():
    if not c["ts"] and not c["ids"]:
        c["ts"] = newest
with open(sys.argv[1], "w") as f:
    json.dump(cursors, f)
PYINIT
  mv "${cursor}.tmp" "$cursor" || return 0

  local wait_pid="" wait_out="${RUN_DIR}/s${idx}.wait" wait_err="${RUN_DIR}/s${idx}.wait.err"
  local read_err="${RUN_DIR}/s${idx}.read.err" outage_since=0 last_err=""
  local last_sweep=-$SWEEP_SECS kind sweep_query sweep_since
  # Own the pending curl as well as the stream: no orphaned long polls on wake.
  trap '[[ -z "$wait_pid" ]] || { kill "$wait_pid" 2>/dev/null; wait "$wait_pid" 2>/dev/null; }' EXIT
  trap 'exit 0' TERM INT

  detect_response() {
    hit="$(printf '%s' "$out" | \
      VM_WAKE_SELF="$VM_MESH_SELF" VM_WAKE_OPERATOR="${VM_MESH_OPERATOR:-}" \
      VM_WAKE_SCOPE="$scope" VM_WAKE_CURSOR_FILE="$cursor" \
      VM_WAKE_LISTEN_FILE="$VM_MESH_LISTEN" VM_WAKE_FROM="$wfrom" VM_WAKE_KEYWORDS="$kws" \
      python3 -c "$DETECT")"; rc=$?
    if (( rc != 0 )); then
      # ANY nonzero rc escalates, with the rc in the text. An ancestor tested
      # rc==3 only, so a missing interpreter (127), OOM (137) or a traceback
      # (1) all read as "no message" — a one-integer-wide guard.
      echo "WATCHER ERROR: ${room} detector exited rc=${rc} (waking to fix — NOT silently treating as no-message)." > "$sig"
      return 1
    fi
    if [[ -n "$hit" ]]; then
      local ts="${hit##*|}" recover="${hit##*|}"
      [[ "$ts" =~ ^[0-9]+$ ]] && recover="$(( ts - 1 ))"
      printf '%s' "$hit" > "$VM_MESH_LASTWAKE" 2>/dev/null || true
      # The empty-drain disambiguation must travel IN the wake line: only this
      # watcher has PROVEN a message exists, so an empty chat_drain after this
      # wake means the subscription is dead, not that nothing arrived.
      # (Observed live 2026-08-12: real wake, drain [], subscriber_count 0.)
      echo "WAKE: new ${room} message from ${hit%%|*} (ts ${ts}). Drain the chat (rooms + DMs), respond, then RE-ARM mesh-watch. IF THE DRAIN COMES BACK EMPTY do NOT conclude there is nothing — this message provably exists; re-subscribe and read ${room} with since=${recover} to recover it.$(registry_countdown)" > "$sig"
      return 1
    fi

    return 0
  }

  # The outage clock (#371): the first failed read starts it, any answer from
  # the daemon stops it. Only OUTAGE_GIVEUP_SECS of continuous failure ends the
  # watcher, and the message carries curl's last error, so a hub outage can be
  # told apart from a watcher fault.
  read_failed() {
    (( outage_since > 0 )) || outage_since=$(date +%s)
    last_err="$(tail -n1 "$1" 2>/dev/null)"
    if (( $(date +%s) - outage_since >= OUTAGE_GIVEUP_SECS )); then
      echo "WATCHER ERROR: ${room} daemon ${VM_MESH_DAEMON} unreachable for $(( $(date +%s) - outage_since ))s (last error: ${last_err:-no response}). Fix the connection, then re-arm." > "$sig"
      return 1
    fi
  }
  read_answered() { outage_since=0; }

  while true; do
    date +%s > "$alive" 2>/dev/null || true
    [[ -f "$VM_MESH_DISARM" ]] && return 0
    (( $(date +%s) - start_ts > MAX_WALL_SECS )) && return 0

    # Exclusive max keeps the combined room/mailbox wait BLOCKING. Overlap
    # belongs to bounded reads, never to the long poll's catch-up query.
    since="$(python3 -c 'import json,sys
with open(sys.argv[1]) as f: cursors=json.load(f)
print(max(c["ts"] for c in cursors.values()))' "$cursor")" || {
      echo "WATCHER ERROR: ${room} cursor unreadable — re-arm." > "$sig"
      return 0
    }
    # curl stays pending while this worker performs the periodic overlap reads.
    # Only this worker ever updates the cursor.
    curl -fsS -m $((WAIT_SECS + 10)) "${VM_MESH_DAEMON}${query}&since=${since}&timeout_secs=${WAIT_SECS}" > "$wait_out" 2>"$wait_err" &
    wait_pid=$!
    last_sweep=-$SWEEP_SECS
    while true; do
      [[ -f "$VM_MESH_DISARM" ]] && return 0
      (( $(date +%s) - start_ts > MAX_WALL_SECS )) && return 0
      if (( SECONDS - last_sweep >= SWEEP_SECS )); then
        for kind in room dm; do
          [[ "$kind" == dm && "$first" != 1 ]] && continue
          sweep_since="$(python3 -c 'import json,sys
with open(sys.argv[1]) as f: cursors=json.load(f)
print(cursors[sys.argv[2]]["ts"]-1)' "$cursor" "$kind")" || {
            echo "WATCHER ERROR: ${room} cursor unreadable — re-arm." > "$sig"
            return 0
          }
          if [[ "$kind" == room ]]; then sweep_query="room=${room}"; else sweep_query="to_agent=${VM_MESH_SELF}"; fi
          out="$(curl -fsS -m 10 "${VM_MESH_DAEMON}/chat_read?${sweep_query}&since=${sweep_since}" 2>"$read_err")"
          if [[ -z "$out" ]]; then
            read_failed "$read_err" || return 0
          else
            read_answered
            detect_response || return 0
          fi
        done
        last_sweep=$SECONDS
      fi
      kill -0 "$wait_pid" 2>/dev/null || break
      sleep 1
    done
    wait "$wait_pid" 2>/dev/null; wait_pid=""
    out="$(cat "$wait_out")"
    date +%s > "$alive" 2>/dev/null || true
    if [[ -z "$out" ]]; then
      # An outage should wake the session ONCE, loudly, after the give-up
      # window: not sit dark for five hours, and not end on a short blip.
      read_failed "$wait_err" || return 0
      sleep "$backoff"
      backoff=$(( backoff * 2 )); (( backoff > BACKOFF_CAP )) && backoff=$BACKOFF_CAP
      continue
    fi
    read_answered; backoff=2

    detect_response || return 0
    # The next arm sweeps immediately after this return, even on timeout.
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

hb_failures=0 exit_reason=""
while true; do
  # Exit conditions first: any signal, wall-clock, or all children dead.
  for ((i=0; i<NSTREAMS; i++)); do
    [[ -f "${RUN_DIR}/s${i}.sig" ]] && { exit_reason=signal; break 2; }
  done
  alive_children=0
  for p in "${PIDS[@]}"; do kill -0 "$p" 2>/dev/null && alive_children=1; done
  elapsed=$(( $(date +%s) - start_ts ))
  # Streams also stop at the ceiling; that is still a quiet heartbeat.
  (( elapsed > MAX_WALL_SECS )) && { exit_reason=wall_clock; break; }
  (( alive_children == 0 )) && { exit_reason=streams_exited; break; }

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
  # Interruptible: bash defers a trap until a foreground sleep returns, but a
  # `wait` returns at once, so a takeover's TERM is handled before its SIGKILL.
  sleep 2 & wait $! 2>/dev/null
done

for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null; done
for p in "${PIDS[@]}"; do wait "$p" 2>/dev/null; done

for ((i=0; i<NSTREAMS; i++)); do
  [[ -f "${RUN_DIR}/s${i}.sig" ]] && { cat "${RUN_DIR}/s${i}.sig"; exit 0; }
done

if [[ -f "$VM_MESH_DISARM" ]]; then echo "DISARMED (sentinel present) — not re-arming."; exit 0; fi
if [[ "$exit_reason" == "streams_exited" ]]; then
  echo "WATCHER ERROR: all streams exited after ${elapsed}s without a message — not a quiet heartbeat. Check the daemon at ${VM_MESH_DAEMON}; re-arm."
  exit 3
fi
if [[ "$exit_reason" == "wall_clock" ]]; then
  echo "RE-ARM: no relevant message within ~${MAX_WALL_SECS}s (quiet heartbeat) — re-launch mesh-watch.sh.$(registry_countdown)"
fi
exit 0
