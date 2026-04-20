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
  out=$(AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1 || true)
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
  out=$(AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
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
  touch "$root/transcripts/new-transcript.jsonl"
  # Backdate by 60s so the age filter treats it as a closed session's transcript
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
  out=$(AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)

  local slot1_status slot2_status settings_variant
  slot1_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['status'])")
  slot2_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][1]['status'])")
  settings_variant=$(python3 -c "import json;print(json.load(open('$root/.claude/settings.local.json')).get('variant',''))")

  assert "slot 1 marked complete" "complete" "$slot1_status"
  assert "slot 2 marked started" "started" "$slot2_status"
  assert "settings.local.json swapped to condition B" "B" "$settings_variant"
  local banner
  banner=$(echo "$out" | grep -c "EXPERIMENT: slot 2/2 ready" || true)
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
  out=$(AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
  local slot1_status
  slot1_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['status'])")
  assert "slot 1 stays started" "started" "$slot1_status"
  local waiting
  waiting=$(echo "$out" | grep -c "EXPERIMENT: slot 1/2 in progress" || true)
  assert "prints in-progress banner" "1" "$waiting"
}

# --- Scenario 5: started slot + FRESH transcript (age < 30s) → in-progress banner
test_started_with_fresh_transcript_waits() {
  echo "Scenario: started slot + fresh (age < 30s) transcript → in-progress banner (no completion)"
  local root
  root=$(setup_fixture)
  touch "$root/experiments/persona-eval/sessions/session-01.snapshot.txt"
  cat > "$root/experiments/persona-eval/sessions/session-01.meta.json" <<'JSON'
{"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null}
JSON
  touch "$root/transcripts/fresh-transcript.jsonl"  # mtime = now, default age filter rejects it
  cat > "$root/experiments/persona-eval/schedule.json" <<'JSON'
{"seed":1,"total_sessions":2,"distribution":{"A":1,"B":1,"C":0},
 "condition_labels":{"A":"full","B":"flat","C":"instr"},
 "slots":[
   {"slot":1,"condition":"A","status":"started","started_at":1.0,"transcript_path":null},
   {"slot":2,"condition":"B","status":"pending","started_at":null,"transcript_path":null}
 ]}
JSON
  local out
  out=$(AUTO_ADVANCE_TRANSCRIPT_DIR="$root/transcripts" "$root/experiments/persona-eval/scripts/auto-advance.sh" 2>&1)
  local slot1_status
  slot1_status=$(python3 -c "import json;print(json.load(open('$root/experiments/persona-eval/schedule.json'))['slots'][0]['status'])")
  assert "slot 1 stays started (fresh transcript rejected)" "started" "$slot1_status"
  local waiting
  waiting=$(echo "$out" | grep -c "EXPERIMENT: slot 1/2 in progress" || true)
  assert "prints in-progress banner" "1" "$waiting"
}

test_no_schedule
test_all_complete
test_started_with_transcript_advances
test_started_without_transcript_noop
test_started_with_fresh_transcript_waits

echo ""
echo "Total: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
