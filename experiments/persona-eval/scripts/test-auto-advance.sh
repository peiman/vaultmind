#!/usr/bin/env bash
# Assertion-style tests for auto-advance.sh. Each scenario builds a fixture
# schedule + transcript dir, runs auto-advance.sh against it, and checks the
# resulting schedule.json state plus any stdout claim.
#
# Fails loud on the first mismatch so the tester sees a clear message.
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PASS=0
FAIL=0

assert() {
  local label="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    echo "  PASS  $label"
    PASS=$((PASS+1))
  else
    echo "  FAIL  $label"
    echo "        expected: $expected"
    echo "        actual:   $actual"
    FAIL=$((FAIL+1))
  fi
}

setup_fixture() {
  local root
  root=$(mktemp -d)
  mkdir -p "$root/experiments/persona-eval/scripts/lib"
  mkdir -p "$root/experiments/persona-eval/sessions"
  mkdir -p "$root/experiments/persona-eval/configs"
  mkdir -p "$root/.claude"
  mkdir -p "$root/transcripts"

  # Minimal valid config files — auto-advance only copies them, doesn't parse.
  echo '{"hooks":{},"variant":"A"}' > "$root/experiments/persona-eval/configs/condition-a-full.json"
  echo '{"hooks":{},"variant":"B"}' > "$root/experiments/persona-eval/configs/condition-b-flat.json"
  echo '{"hooks":{},"variant":"C"}' > "$root/experiments/persona-eval/configs/condition-c-instruction.json"

  cp "$SCRIPT_DIR/lib/find-new-transcript.py" "$root/experiments/persona-eval/scripts/lib/"
  cp "$SCRIPT_DIR/auto-advance.sh" "$root/experiments/persona-eval/scripts/"
  chmod +x "$root/experiments/persona-eval/scripts/lib/find-new-transcript.py"
  chmod +x "$root/experiments/persona-eval/scripts/auto-advance.sh"

  echo "$root"
}

# --- Scenario 1: no schedule.json → silent no-op
test_no_schedule() {
  echo "Scenario: no schedule.json → silent no-op"
  local root
  root=$(setup_fixture)
  local out
  out=$(AUTO_ADVANCE_NO_NOTIFY=1 AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1 || true)
  assert "no-op output" "" "$out"
}

# --- Scenario 2: all slots complete → prints done message
test_all_complete() {
  echo "Scenario: all slots already complete → done message"
  local root
  root=$(setup_fixture)
  cat > "$root/experiments/persona-eval/schedule.json" <<'JSON'
{"seed":1,"total_sessions":2,"distribution":{"A":1,"B":1,"C":0},
 "condition_labels":{"A":"full","B":"flat","C":"instr"},
 "slots":[
   {"slot":1,"condition":"A","status":"complete","started_at":1.0,"transcript_path":"/x.jsonl"},
   {"slot":2,"condition":"B","status":"complete","started_at":2.0,"transcript_path":"/y.jsonl"}
 ]}
JSON
  local out
  out=$(AUTO_ADVANCE_NO_NOTIFY=1 AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
  local banner
  banner=$(echo "$out" | grep -c "EXPERIMENT COMPLETE" || true)
  assert "prints EXPERIMENT COMPLETE" "1" "$banner"
}

# --- Scenario 3: started slot + new transcript → auto-complete + advance
test_started_with_transcript_advances() {
  echo "Scenario: started slot + new transcript → auto-complete + advance"
  local root
  root=$(setup_fixture)
  # Empty snapshot → every current transcript counts as new
  touch "$root/experiments/persona-eval/sessions/session-01.snapshot.txt"
  cat > "$root/experiments/persona-eval/sessions/session-01.meta.json" <<'JSON'
{"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null}
JSON
  # Transcript must have ≥1 user message to pass the empty-window filter.
  cat > "$root/transcripts/new-transcript.jsonl" <<'JSONL'
{"type":"user","message":{"content":"hi"}}
{"type":"assistant","message":{"content":"hi"}}
JSONL
  # Backdate by 60s so older timestamp is plausible for "closed prior session".
  python3 -c "
import os, time
p='$root/transcripts/new-transcript.jsonl'
past = time.time() - 60
os.utime(p, (past, past))
"
  cat > "$root/experiments/persona-eval/schedule.json" <<'JSON'
{"seed":1,"total_sessions":2,"distribution":{"A":1,"B":1,"C":0},
 "condition_labels":{"A":"full","B":"flat","C":"instr"},
 "slots":[
   {"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null},
   {"slot":2,"condition":"B","status":"pending","started_at":null,"transcript_path":null}
 ]}
JSON
  local out
  out=$(AUTO_ADVANCE_NO_NOTIFY=1 AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)

  local slot1_status slot2_status settings_variant
  slot1_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['status'])")
  slot2_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][1]['status'])")
  settings_variant=$(python3 -c "import json;print(json.load(open('$root/.claude/settings.local.json')).get('variant',''))")

  assert "slot 1 marked complete" "complete" "$slot1_status"
  assert "slot 2 marked started" "started" "$slot2_status"
  assert "settings.local.json swapped to condition B" "B" "$settings_variant"
  local banner
  banner=$(echo "$out" | grep -c "Slot 2/2 READY" || true)
  assert "prints next-slot banner" "1" "$banner"
}

# --- Scenario 4: started slot + no new transcript → silent wait with banner
test_started_without_transcript_noop() {
  echo "Scenario: started slot + no new transcript → in-progress banner"
  local root
  root=$(setup_fixture)
  touch "$root/transcripts/old.jsonl"
  echo "$root/transcripts/old.jsonl" > "$root/experiments/persona-eval/sessions/session-01.snapshot.txt"
  cat > "$root/experiments/persona-eval/sessions/session-01.meta.json" <<'JSON'
{"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null}
JSON
  cat > "$root/experiments/persona-eval/schedule.json" <<'JSON'
{"seed":1,"total_sessions":2,"distribution":{"A":1,"B":1,"C":0},
 "condition_labels":{"A":"full","B":"flat","C":"instr"},
 "slots":[
   {"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null},
   {"slot":2,"condition":"B","status":"pending","started_at":null,"transcript_path":null}
 ]}
JSON
  local out
  out=$(AUTO_ADVANCE_NO_NOTIFY=1 AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
  local slot1_status
  slot1_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['status'])")
  assert "slot 1 stays started" "started" "$slot1_status"
  local waiting
  waiting=$(echo "$out" | grep -c "ACTIVE (condition A)" || true)
  assert "prints ACTIVE work-session banner" "1" "$waiting"
}

#   (old scenario 5 replaced by scenario 5b below — fresh transcripts are no
#    longer rejected by age; they're excluded only when they ARE the current
#    session's transcript, identified via stdin JSON from Claude Code.)

# --- Scenario 5b: started slot + fresh transcript that IS current session → waits
test_excludes_current_session_transcript() {
  echo "Scenario: started slot + fresh transcript that IS current session → in-progress"
  local root
  root=$(setup_fixture)
  touch "$root/experiments/persona-eval/sessions/session-01.snapshot.txt"
  cat > "$root/experiments/persona-eval/sessions/session-01.meta.json" <<'JSON'
{"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null}
JSON
  # A "real" transcript with 1 user message, and it IS the current session.
  cat > "$root/transcripts/current.jsonl" <<'JSONL'
{"type":"user","message":{"content":"hi"}}
{"type":"assistant","message":{"content":"hi back"}}
JSONL
  # Backdate so the old 30s age filter would NOT save us — only path exclusion can.
  python3 -c "import os,time; os.utime('$root/transcripts/current.jsonl',(time.time()-120,time.time()-120))"
  cat > "$root/experiments/persona-eval/schedule.json" <<'JSON'
{"seed":1,"total_sessions":2,"distribution":{"A":1,"B":1,"C":0},
 "condition_labels":{"A":"full","B":"flat","C":"instr"},
 "slots":[
   {"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null},
   {"slot":2,"condition":"B","status":"pending","started_at":null,"transcript_path":null}
 ]}
JSON
  local out
  out=$(echo "{\"transcript_path\":\"$root/transcripts/current.jsonl\",\"session_id\":\"test\",\"hook_event_name\":\"SessionStart\",\"source\":\"startup\"}" \
    | AUTO_ADVANCE_NO_NOTIFY=1 AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
  local slot1_status
  slot1_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['status'])")
  assert "slot 1 stays started (current session excluded via stdin)" "started" "$slot1_status"
}

# --- Scenario 6: new transcript with 0 user messages → filtered, still in progress
test_empty_window_filtered() {
  echo "Scenario: started slot + transcript with 0 user messages → in-progress"
  local root
  root=$(setup_fixture)
  touch "$root/experiments/persona-eval/sessions/session-01.snapshot.txt"
  cat > "$root/experiments/persona-eval/sessions/session-01.meta.json" <<'JSON'
{"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null}
JSON
  cat > "$root/transcripts/perms-only.jsonl" <<'JSONL'
{"type":"permission-mode","permissionMode":"plan"}
{"type":"permission-mode","permissionMode":"acceptEdits"}
{"type":"attachment","something":"else"}
JSONL
  python3 -c "import os,time; os.utime('$root/transcripts/perms-only.jsonl',(time.time()-60,time.time()-60))"
  cat > "$root/experiments/persona-eval/schedule.json" <<'JSON'
{"seed":1,"total_sessions":2,"distribution":{"A":1,"B":1,"C":0},
 "condition_labels":{"A":"full","B":"flat","C":"instr"},
 "slots":[
   {"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null},
   {"slot":2,"condition":"B","status":"pending","started_at":null,"transcript_path":null}
 ]}
JSON
  local out
  out=$(AUTO_ADVANCE_NO_NOTIFY=1 AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
  local slot1_status
  slot1_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['status'])")
  assert "slot 1 stays started (empty-window filtered)" "started" "$slot1_status"
}

# --- Scenario 7: multiple new eligible transcripts → pick the OLDEST (work session first)
test_picks_oldest_eligible_transcript() {
  echo "Scenario: multiple new eligible transcripts → pick the oldest"
  local root
  root=$(setup_fixture)
  touch "$root/experiments/persona-eval/sessions/session-01.snapshot.txt"
  cat > "$root/experiments/persona-eval/sessions/session-01.meta.json" <<'JSON'
{"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null}
JSON
  cat > "$root/transcripts/older.jsonl" <<'JSONL'
{"type":"user","message":{"content":"hi"}}
{"type":"assistant","message":{"content":"hi"}}
JSONL
  cat > "$root/transcripts/newer.jsonl" <<'JSONL'
{"type":"user","message":{"content":"later"}}
{"type":"assistant","message":{"content":"ack"}}
JSONL
  # Older has mtime 5 minutes ago; newer has mtime 2 minutes ago.
  python3 -c "
import os, time
now = time.time()
os.utime('$root/transcripts/older.jsonl', (now - 300, now - 300))
os.utime('$root/transcripts/newer.jsonl', (now - 120, now - 120))
"
  cat > "$root/experiments/persona-eval/schedule.json" <<'JSON'
{"seed":1,"total_sessions":2,"distribution":{"A":1,"B":1,"C":0},
 "condition_labels":{"A":"full","B":"flat","C":"instr"},
 "slots":[
   {"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null},
   {"slot":2,"condition":"B","status":"pending","started_at":null,"transcript_path":null}
 ]}
JSON
  local out
  out=$(AUTO_ADVANCE_NO_NOTIFY=1 AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
  local claimed
  claimed=$(python3 -c "import json,os;print(os.path.basename(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['transcript_path']))")
  assert "slot 1 claimed oldest transcript (older.jsonl)" "older.jsonl" "$claimed"
}

test_no_schedule
test_all_complete
test_started_with_transcript_advances
test_started_without_transcript_noop
test_excludes_current_session_transcript
test_empty_window_filtered
test_picks_oldest_eligible_transcript

echo ""
echo "Total: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
