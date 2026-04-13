# Persona Eval Blinded Measurement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the tooling for a 20-session blinded experiment measuring whether VaultMind identity injection produces detectable behavioral differences.

**Architecture:** Shell scripts manage session lifecycle (schedule, config swap, transcript discovery). A shell scorer sends transcripts to configurable LLM APIs. A Python script does statistical analysis. Three static hook config variants define the conditions.

**Tech Stack:** Bash, jq, curl (session management + scoring), Python 3 with numpy/scipy/pandas (analysis)

**Spec:** `docs/som/2026-04-11-persona-evaluation/experiment/protocol-design.md`

---

### Task 1: Create directory structure and README

**Files:**
- Create: `experiments/persona-eval/README.md`
- Create: `experiments/persona-eval/configs/` (directory)
- Create: `experiments/persona-eval/scripts/` (directory)
- Create: `experiments/persona-eval/rubric.md`

- [ ] **Step 1: Create directories**

```bash
mkdir -p experiments/persona-eval/{configs,scripts,sessions,results}
```

- [ ] **Step 2: Write the rubric**

Create `experiments/persona-eval/rubric.md`:

```markdown
# Persona Injection Scoring Rubric

Score this Claude Code session transcript. You have no information about what
configuration produced this session. Score strictly from the text.

## Pass 1: Turn 1 Only

Score ONLY the agent's first response to the user's opening message.

| Signal | Score | Criteria |
|--------|-------|----------|
| Identity greeting | 0 or 1 | Uses the user's name or references an existing working relationship. 0 = generic greeting ("How can I help?", "What would you like to work on?"). 1 = personalized. |
| Unprompted vault content | 0 or 1 | References specific project state, growth arcs, design principles, or identity concepts without being asked. 0 = no specific references. 1 = references specific content. |
| Communication style | 0-2 | 0 = generic assistant tone (hedging, formal, deferential). 1 = competent but impersonal (direct but no shared context assumed). 2 = partner tone (direct, no hedging, assumes shared context, picks up as if continuing prior work). |

**Turn 1 max: 4 points.**

## Pass 2: Full Transcript

Score across the ENTIRE session, including turn 1.

| Signal | Score | Criteria |
|--------|-------|----------|
| Project fact accuracy | 0-3 | Agent gets verifiable project facts right without being told. 0 = none or wrong. 1 = vague ("you're working on a memory system"). 2 = mostly right ("VaultMind uses hybrid retrieval"). 3 = specific and correct ("BGE-M3 with 4-way RRF, ORT backend"). |
| Partner communication style | 0-3 | Sustained partner-mode across the session. 0 = assistant mode throughout. 1 = occasional flashes of directness. 2 = mostly direct and collaborative. 3 = consistent partner tone, challenges assumptions, shows initiative. |
| Unprompted vault references | 0-3 | References vault concepts (arcs, principles, decisions, project history) during natural work. 0 = never. 1 = once. 2 = several times. 3 = woven into reasoning throughout. |
| Latency to domain relevance | 0-2 | How quickly the agent makes a domain-relevant statement (about VaultMind, memory systems, the codebase). 0 = never without prompting. 1 = after the user prompted domain context. 2 = within first few turns unprompted. |

**Full transcript max: 11 points.**

## Output Format

Return ONLY valid JSON matching this schema:

```json
{
  "turn1": {
    "identity_greeting": 0,
    "unprompted_vault_content": 0,
    "communication_style": 0,
    "total": 0,
    "evidence": ["quote the specific text that justified each score"]
  },
  "full_transcript": {
    "project_fact_accuracy": 0,
    "partner_communication_style": 0,
    "unprompted_vault_references": 0,
    "latency_to_domain_relevance": 0,
    "total": 0,
    "evidence": ["quote the specific text that justified each score"]
  }
}
```
```

- [ ] **Step 3: Write the README**

Create `experiments/persona-eval/README.md`:

```markdown
# Persona Injection Blinded Measurement

Layer 1 experiment: does VaultMind identity injection produce detectable
behavioral differences in Claude Code sessions?

## Quick Start

```bash
# 1. One-time setup: generate schedule and capture flat-paste content
bash experiments/persona-eval/scripts/generate-schedule.sh
bash experiments/persona-eval/scripts/capture-flat-paste.sh

# 2. Before each session (20 times)
bash experiments/persona-eval/scripts/start-session.sh

# 3. After all 20 sessions: score with one or more LLMs
bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o
bash experiments/persona-eval/scripts/score-transcripts.sh --llm google/gemini-pro

# 4. Analyze
python3 experiments/persona-eval/scripts/analyze.py
# Output: experiments/persona-eval/results/report.md
```

## Conditions

- **A (Full injection):** Production hook runs VaultMind retrieval
- **B (Flat paste):** Same content hardcoded, no retrieval
- **C (Instruction only):** One-line project description

## Design

See `docs/som/2026-04-11-persona-evaluation/experiment/protocol-design.md`
```

- [ ] **Step 4: Commit**

```bash
git add experiments/persona-eval/
git commit -m "feat(persona-eval): scaffold experiment directory with rubric and README"
```

---

### Task 2: Create hook config variants

**Files:**
- Create: `experiments/persona-eval/configs/condition-a-full.json`
- Create: `experiments/persona-eval/configs/condition-b-flat.json`
- Create: `experiments/persona-eval/configs/condition-c-instruction.json`
- Create: `experiments/persona-eval/scripts/capture-flat-paste.sh`

These are `.claude/settings.local.json` variants. Each includes the existing permissions block (copied from the current file) plus a condition-specific SessionStart hook. The PreToolUse hook (commit message check) is preserved in all variants.

- [ ] **Step 1: Read the current settings.local.json to get the permissions block**

```bash
cat .claude/settings.local.json
```

Save the `"permissions"` and `"disabledMcpjsonServers"` sections — they'll be reused in all three variants.

- [ ] **Step 2: Create condition-a-full.json**

This variant is identical to the current production `settings.local.json` — the SessionStart hook runs the vault load-persona script.

Create `experiments/persona-eval/configs/condition-a-full.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/.ckeletin/scripts/install_tools.sh"
          },
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/load-persona.sh"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/.ckeletin/scripts/check-commit-msg.sh"
          }
        ]
      }
    ]
  },
  "permissions": { ... COPY FROM CURRENT settings.local.json ... },
  "disabledMcpjsonServers": [ ... COPY FROM CURRENT settings.local.json ... ]
}
```

- [ ] **Step 3: Create the flat-paste hook script**

Create `experiments/persona-eval/scripts/inject-flat-paste.sh`:

```bash
#!/bin/bash
# Inject hardcoded flat-paste content for condition B.
# Content is captured once from a real vault run via capture-flat-paste.sh.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FLAT_FILE="$SCRIPT_DIR/../configs/flat-paste-content.txt"

if [ -f "$FLAT_FILE" ]; then
  cat "$FLAT_FILE"
fi
```

- [ ] **Step 4: Create condition-b-flat.json**

Same as condition-a, but replaces the load-persona hook with the flat-paste script:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/.ckeletin/scripts/install_tools.sh"
          },
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/experiments/persona-eval/scripts/inject-flat-paste.sh"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/.ckeletin/scripts/check-commit-msg.sh"
          }
        ]
      }
    ]
  },
  "permissions": { ... SAME AS CONDITION A ... },
  "disabledMcpjsonServers": [ ... SAME AS CONDITION A ... ]
}
```

- [ ] **Step 5: Create the instruction-only hook script**

Create `experiments/persona-eval/scripts/inject-instruction.sh`:

```bash
#!/bin/bash
# Inject minimal instruction for condition C.
echo "You are working with Peiman on VaultMind, a memory system for AI agents. The codebase is a Go CLI project."
```

- [ ] **Step 6: Create condition-c-instruction.json**

Same structure, replaces the persona hook with the instruction script:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/.ckeletin/scripts/install_tools.sh"
          },
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/experiments/persona-eval/scripts/inject-instruction.sh"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "bash \"$CLAUDE_PROJECT_DIR\"/.ckeletin/scripts/check-commit-msg.sh"
          }
        ]
      }
    ]
  },
  "permissions": { ... SAME AS CONDITION A ... },
  "disabledMcpjsonServers": [ ... SAME AS CONDITION A ... ]
}
```

- [ ] **Step 7: Create capture-flat-paste.sh**

This runs once during setup to capture the real vault output for condition B.

Create `experiments/persona-eval/scripts/capture-flat-paste.sh`:

```bash
#!/bin/bash
# Capture a real vault injection output for use as the flat-paste condition.
# Run this once during experiment setup, before session 1.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUTPUT="$SCRIPT_DIR/../configs/flat-paste-content.txt"

VAULTMIND="/tmp/vaultmind"
VAULT_PATH="$PROJECT_DIR/vaultmind-identity"

# Build if needed
if [ ! -f "$VAULTMIND" ]; then
  echo "Building vaultmind..."
  (cd "$PROJECT_DIR" && go build -o "$VAULTMIND" .)
fi

if [ ! -f "$VAULTMIND" ] || [ ! -d "$VAULT_PATH" ]; then
  echo "ERROR: vaultmind binary or vault not found"
  exit 1
fi

IDENTITY=$("$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000)
CONTEXT=$("$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 3 --budget 2000)

{
  echo "IDENTITY CONTEXT:"
  echo ""
  echo "$IDENTITY"
  echo ""
  echo "CURRENT CONTEXT:"
  echo ""
  echo "$CONTEXT"
} > "$OUTPUT"

CHARS=$(wc -c < "$OUTPUT")
echo "Captured flat-paste content: $CHARS bytes -> $OUTPUT"
echo "Review it to confirm it looks right, then proceed with generate-schedule.sh"
```

- [ ] **Step 8: Make scripts executable and commit**

```bash
chmod +x experiments/persona-eval/scripts/capture-flat-paste.sh
chmod +x experiments/persona-eval/scripts/inject-flat-paste.sh
chmod +x experiments/persona-eval/scripts/inject-instruction.sh
git add experiments/persona-eval/
git commit -m "feat(persona-eval): hook config variants for 3 conditions"
```

---

### Task 3: Generate schedule script

**Files:**
- Create: `experiments/persona-eval/scripts/generate-schedule.sh`

- [ ] **Step 1: Write generate-schedule.sh**

Create `experiments/persona-eval/scripts/generate-schedule.sh`:

```bash
#!/bin/bash
# Generate a randomized 20-session schedule for the persona eval experiment.
# Creates schedule.json with shuffled condition assignments.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"

if [ -f "$SCHEDULE" ]; then
  echo "ERROR: schedule.json already exists. Delete it first to regenerate."
  exit 1
fi

# Generate seed from current timestamp
SEED=$(date +%s)

# Create 20 slots: 7xA, 7xB, 6xC — shuffle with recorded seed
python3 -c "
import json, random

seed = $SEED
random.seed(seed)

conditions = ['A'] * 7 + ['B'] * 7 + ['C'] * 6
random.shuffle(conditions)

schedule = {
    'seed': seed,
    'total_sessions': 20,
    'distribution': {'A': 7, 'B': 7, 'C': 6},
    'condition_labels': {
        'A': 'full_injection',
        'B': 'flat_paste',
        'C': 'instruction_only'
    },
    'slots': []
}

for i, cond in enumerate(conditions, 1):
    schedule['slots'].append({
        'slot': i,
        'condition': cond,
        'status': 'pending',
        'started_at': None,
        'transcript_path': None
    })

print(json.dumps(schedule, indent=2))
" > "$SCHEDULE"

echo "Schedule generated with seed $SEED -> $SCHEDULE"
echo "Distribution:"
python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    print(f\"  Session {slot['slot']:2d}: Condition {slot['condition']}\")
"
echo ""
echo "WARNING: Do not look at this file during data collection."
echo "Next: run capture-flat-paste.sh, then start-session.sh for each session."
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x experiments/persona-eval/scripts/generate-schedule.sh
git add experiments/persona-eval/scripts/generate-schedule.sh
git commit -m "feat(persona-eval): schedule generation script"
```

---

### Task 4: Start session script

**Files:**
- Create: `experiments/persona-eval/scripts/start-session.sh`

- [ ] **Step 1: Write start-session.sh**

Create `experiments/persona-eval/scripts/start-session.sh`:

```bash
#!/bin/bash
# Start the next experiment session.
# Checks if previous session completed, swaps hook config, logs start.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_DIR="$(cd "$EXPERIMENT_DIR/../.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"
SESSIONS_DIR="$EXPERIMENT_DIR/sessions"
SETTINGS_FILE="$PROJECT_DIR/.claude/settings.local.json"
BACKUP_FILE="$EXPERIMENT_DIR/.settings-backup.json"

TRANSCRIPT_DIR="$HOME/.claude/projects/-Users-peiman-dev-cli-vaultmind"

if [ ! -f "$SCHEDULE" ]; then
  echo "ERROR: No schedule.json found. Run generate-schedule.sh first."
  exit 1
fi

mkdir -p "$SESSIONS_DIR"

# --- Auto-complete previous session if transcript exists ---
LAST_STARTED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
last = None
for slot in s['slots']:
    if slot['status'] == 'started':
        last = slot['slot']
print(last if last else '')
")

if [ -n "$LAST_STARTED" ]; then
  META_FILE="$SESSIONS_DIR/session-$(printf '%02d' "$LAST_STARTED").meta.json"
  STARTED_AT=$(python3 -c "
import json
m = json.load(open('$META_FILE'))
print(m['started_at'])
")

  # Find the most recent transcript created after the session started
  TRANSCRIPT=$(python3 -c "
import os, glob

transcript_dir = '$TRANSCRIPT_DIR'
started_at = float('$STARTED_AT')
candidates = []

for f in glob.glob(os.path.join(transcript_dir, '*.jsonl')):
    mtime = os.path.getmtime(f)
    if mtime > started_at:
        candidates.append((mtime, f))

if candidates:
    candidates.sort(reverse=True)
    print(candidates[0][1])
else:
    print('')
")

  if [ -n "$TRANSCRIPT" ]; then
    # Mark previous session as complete
    python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['slot'] == $LAST_STARTED:
        slot['status'] = 'complete'
        slot['transcript_path'] = '$TRANSCRIPT'
        break
with open('$SCHEDULE', 'w') as f:
    json.dump(s, f, indent=2)
"
    # Update meta file
    python3 -c "
import json
m = json.load(open('$META_FILE'))
m['transcript_path'] = '$TRANSCRIPT'
m['status'] = 'complete'
with open('$META_FILE', 'w') as f:
    json.dump(m, f, indent=2)
"
    echo "Previous session $LAST_STARTED completed (transcript found)."
  else
    echo "WARNING: Session $LAST_STARTED was started but no transcript found."
    echo "Re-using slot $LAST_STARTED."
    # Reset to pending so it gets picked up again
    python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['slot'] == $LAST_STARTED:
        slot['status'] = 'pending'
        break
with open('$SCHEDULE', 'w') as f:
    json.dump(s, f, indent=2)
"
  fi
fi

# --- Check if all sessions are done ---
REMAINING=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
pending = [sl for sl in s['slots'] if sl['status'] == 'pending']
print(len(pending))
")

if [ "$REMAINING" -eq 0 ]; then
  COMPLETED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
done = [sl for sl in s['slots'] if sl['status'] == 'complete']
print(len(done))
")
  echo "All $COMPLETED sessions complete! Time to score."
  echo "Run: bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o"

  # Restore original settings
  if [ -f "$BACKUP_FILE" ]; then
    cp "$BACKUP_FILE" "$SETTINGS_FILE"
    echo "Original settings.local.json restored."
  fi
  exit 0
fi

# --- Find next pending slot ---
NEXT_SLOT=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['status'] == 'pending':
        print(slot['slot'])
        break
")

CONDITION=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['slot'] == $NEXT_SLOT:
        print(slot['condition'])
        break
")

TOTAL=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
done = len([sl for sl in s['slots'] if sl['status'] == 'complete'])
print(done + 1)
")

# --- Backup current settings and swap in condition config ---
if [ ! -f "$BACKUP_FILE" ]; then
  cp "$SETTINGS_FILE" "$BACKUP_FILE"
fi

# Map condition to config file
case "$CONDITION" in
  A) CONFIG="$EXPERIMENT_DIR/configs/condition-a-full.json" ;;
  B) CONFIG="$EXPERIMENT_DIR/configs/condition-b-flat.json" ;;
  C) CONFIG="$EXPERIMENT_DIR/configs/condition-c-instruction.json" ;;
esac

cp "$CONFIG" "$SETTINGS_FILE"

# --- Mark slot as started ---
START_TIME=$(python3 -c "import time; print(time.time())")

python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['slot'] == $NEXT_SLOT:
        slot['status'] = 'started'
        slot['started_at'] = '$START_TIME'
        break
with open('$SCHEDULE', 'w') as f:
    json.dump(s, f, indent=2)
"

# --- Write session metadata ---
META_FILE="$SESSIONS_DIR/session-$(printf '%02d' "$NEXT_SLOT").meta.json"
python3 -c "
import json
meta = {
    'slot': $NEXT_SLOT,
    'condition': '$CONDITION',
    'status': 'started',
    'started_at': '$START_TIME',
    'transcript_path': None
}
with open('$META_FILE', 'w') as f:
    json.dump(meta, f, indent=2)
"

echo ""
echo "Session $TOTAL/20 ready. Open a new Claude Code session in this project."
echo ""
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x experiments/persona-eval/scripts/start-session.sh
git add experiments/persona-eval/scripts/start-session.sh
git commit -m "feat(persona-eval): session start script with auto-completion"
```

---

### Task 5: Transcript scorer script

**Files:**
- Create: `experiments/persona-eval/scripts/score-transcripts.sh`

- [ ] **Step 1: Write score-transcripts.sh**

Create `experiments/persona-eval/scripts/score-transcripts.sh`:

```bash
#!/bin/bash
# Score experiment transcripts using a configurable LLM rater.
# Usage: score-transcripts.sh --llm openai/gpt-4o
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"
RUBRIC="$EXPERIMENT_DIR/rubric.md"
RESULTS_DIR="$EXPERIMENT_DIR/results"

# --- Parse arguments ---
LLM=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --llm) LLM="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [ -z "$LLM" ]; then
  echo "Usage: score-transcripts.sh --llm provider/model"
  echo "Examples:"
  echo "  --llm openai/gpt-4o"
  echo "  --llm anthropic/claude-sonnet-4-6"
  echo "  --llm google/gemini-2.0-flash"
  exit 1
fi

PROVIDER="${LLM%%/*}"
MODEL="${LLM#*/}"
MODEL_SLUG=$(echo "$MODEL" | tr '.' '-' | tr '/' '-')
TIMESTAMP=$(date +%Y%m%dT%H%M%S)
OUTPUT_FILE="$RESULTS_DIR/scores-${MODEL_SLUG}-${TIMESTAMP}.json"

mkdir -p "$RESULTS_DIR"

# --- Resolve API key and endpoint ---
case "$PROVIDER" in
  openai)
    API_KEY="${OPENAI_API_KEY:?Set OPENAI_API_KEY}"
    API_URL="https://api.openai.com/v1/chat/completions"
    ;;
  anthropic)
    API_KEY="${ANTHROPIC_API_KEY:?Set ANTHROPIC_API_KEY}"
    API_URL="https://api.anthropic.com/v1/messages"
    ;;
  google)
    API_KEY="${GOOGLE_API_KEY:?Set GOOGLE_API_KEY}"
    API_URL="https://generativelanguage.googleapis.com/v1beta/models/${MODEL}:generateContent?key=${API_KEY}"
    ;;
  *)
    echo "Unsupported provider: $PROVIDER (supported: openai, anthropic, google)"
    exit 1
    ;;
esac

RUBRIC_TEXT=$(cat "$RUBRIC")

# --- Extract conversation text from a JSONL transcript ---
extract_conversation() {
  local transcript_path="$1"
  python3 -c "
import json, sys

turns = []
with open('$transcript_path') as f:
    for line in f:
        d = json.loads(line)
        t = d.get('type', '')

        if t == 'user' and isinstance(d.get('message'), dict):
            content = d['message'].get('content', '')
            if isinstance(content, str) and not content.startswith('<'):
                turns.append(f'USER: {content}')
            elif isinstance(content, str) and '<bash-input>' in content:
                # Skip system-generated messages
                pass

        elif t == 'assistant':
            msg = d.get('message', {})
            if isinstance(msg, dict):
                content = msg.get('content', [])
                if isinstance(content, list):
                    text_parts = []
                    for block in content:
                        if isinstance(block, dict) and block.get('type') == 'text':
                            text_parts.append(block['text'])
                    if text_parts:
                        combined = '\n'.join(text_parts)
                        turns.append(f'ASSISTANT: {combined}')

# Deduplicate consecutive identical assistant turns (streaming artifacts)
deduped = []
for turn in turns:
    if not deduped or turn != deduped[-1]:
        deduped.append(turn)

print('\n\n'.join(deduped))
"
}

# --- Call LLM API ---
# Uses temp files for payloads to handle large transcripts safely.
call_llm() {
  local system_prompt="$1"
  local user_prompt="$2"
  local payload_file="$TMPDIR_SCORE/payload.json"
  local response_file="$TMPDIR_SCORE/response.json"

  # Write system and user prompts to temp files for safe JSON encoding
  local sys_file="$TMPDIR_SCORE/sys_prompt.txt"
  local usr_file="$TMPDIR_SCORE/usr_prompt.txt"
  printf '%s' "$system_prompt" > "$sys_file"
  printf '%s' "$user_prompt" > "$usr_file"

  case "$PROVIDER" in
    openai)
      python3 -c "
import json
with open('$sys_file') as f: sys_p = f.read()
with open('$usr_file') as f: usr_p = f.read()
payload = {
    'model': '$MODEL',
    'messages': [
        {'role': 'system', 'content': sys_p},
        {'role': 'user', 'content': usr_p}
    ],
    'temperature': 0,
    'response_format': {'type': 'json_object'}
}
with open('$payload_file', 'w') as f:
    json.dump(payload, f)
"
      curl -s "$API_URL" \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d @"$payload_file" > "$response_file"

      python3 -c "
import json
with open('$response_file') as f: r = json.load(f)
print(r['choices'][0]['message']['content'])
"
      ;;

    anthropic)
      python3 -c "
import json
with open('$sys_file') as f: sys_p = f.read()
with open('$usr_file') as f: usr_p = f.read()
payload = {
    'model': '$MODEL',
    'max_tokens': 4096,
    'system': sys_p,
    'messages': [
        {'role': 'user', 'content': usr_p}
    ]
}
with open('$payload_file', 'w') as f:
    json.dump(payload, f)
"
      curl -s "$API_URL" \
        -H "x-api-key: $API_KEY" \
        -H "anthropic-version: 2023-06-01" \
        -H "Content-Type: application/json" \
        -d @"$payload_file" > "$response_file"

      python3 -c "
import json
with open('$response_file') as f: r = json.load(f)
print(r['content'][0]['text'])
"
      ;;

    google)
      python3 -c "
import json
with open('$sys_file') as f: sys_p = f.read()
with open('$usr_file') as f: usr_p = f.read()
payload = {
    'contents': [{'parts': [{'text': sys_p + '\n\n' + usr_p}]}],
    'generationConfig': {'temperature': 0, 'responseMimeType': 'application/json'}
}
with open('$payload_file', 'w') as f:
    json.dump(payload, f)
"
      curl -s "$API_URL" \
        -H "Content-Type: application/json" \
        -d @"$payload_file" > "$response_file"

      python3 -c "
import json
with open('$response_file') as f: r = json.load(f)
print(r['candidates'][0]['content']['parts'][0]['text'])
"
      ;;
  esac
}

# --- Score each completed session ---
echo "Scoring transcripts with $LLM..."

SYSTEM_PROMPT="You are a behavioral scoring rater. Score the following Claude Code session transcript using the provided rubric. Return ONLY valid JSON matching the schema in the rubric. Be strict. Quote specific text as evidence."

# Use temp files for large content — shell variables break on long transcripts
TMPDIR_SCORE=$(mktemp -d)
RESULTS_FILE="$TMPDIR_SCORE/results.json"
echo "[]" > "$RESULTS_FILE"
trap "rm -rf $TMPDIR_SCORE" EXIT

COMPLETED_SLOTS=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['status'] == 'complete' and slot.get('transcript_path'):
        print(f\"{slot['slot']}|{slot['transcript_path']}\")
")

SCORED=0
TOTAL=$(echo "$COMPLETED_SLOTS" | grep -c '|' || echo 0)

while IFS='|' read -r SLOT TRANSCRIPT_PATH; do
  [ -z "$SLOT" ] && continue
  SCORED=$((SCORED + 1))
  echo "  Scoring session $SLOT ($SCORED/$TOTAL)..."

  # Write conversation to temp file to avoid shell variable limits
  CONV_FILE="$TMPDIR_SCORE/conversation.txt"
  extract_conversation "$TRANSCRIPT_PATH" > "$CONV_FILE"

  # Build the user prompt in a temp file
  PROMPT_FILE="$TMPDIR_SCORE/prompt.txt"
  {
    cat "$RUBRIC"
    echo ""
    echo "---"
    echo ""
    echo "## Transcript to Score"
    echo ""
    cat "$CONV_FILE"
  } > "$PROMPT_FILE"

  SCORE_FILE="$TMPDIR_SCORE/score.json"
  call_llm "$SYSTEM_PROMPT" "$(cat "$PROMPT_FILE")" > "$SCORE_FILE"

  # Append to results
  python3 -c "
import json
with open('$RESULTS_FILE') as f:
    results = json.load(f)
try:
    with open('$SCORE_FILE') as f:
        score = json.loads(f.read())
except (json.JSONDecodeError, Exception) as e:
    score = {'error': f'Failed to parse LLM response: {e}'}
results.append({
    'slot': $SLOT,
    'scores': score
})
with open('$RESULTS_FILE', 'w') as f:
    json.dump(results, f)
"

done <<< "$COMPLETED_SLOTS"

# --- Write output ---
python3 -c "
import json
with open('$RESULTS_FILE') as f:
    results = json.load(f)
output = {
    'llm': '$LLM',
    'model': '$MODEL',
    'timestamp': '$TIMESTAMP',
    'sessions_scored': $SCORED,
    'scores': results
}
with open('$OUTPUT_FILE', 'w') as f:
    json.dump(output, f, indent=2)
"

echo ""
echo "Scoring complete: $SCORED sessions scored."
echo "Results: $OUTPUT_FILE"
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x experiments/persona-eval/scripts/score-transcripts.sh
git add experiments/persona-eval/scripts/score-transcripts.sh
git commit -m "feat(persona-eval): transcript scorer with configurable LLM rater"
```

---

### Task 6: Analysis script

**Files:**
- Create: `experiments/persona-eval/scripts/analyze.py`

- [ ] **Step 1: Write analyze.py**

Create `experiments/persona-eval/scripts/analyze.py`:

```python
#!/usr/bin/env python3
"""Analyze persona eval experiment results.

Reads score files from results/, cross-references with schedule.json,
produces a markdown report with per-condition stats, effect sizes,
and inter-rater agreement.

Usage:
    python3 experiments/persona-eval/scripts/analyze.py
    python3 experiments/persona-eval/scripts/analyze.py --all-runs
"""

import argparse
import glob
import json
import os
import sys
from collections import defaultdict
from pathlib import Path

import numpy as np
import pandas as pd
from scipy import stats

SCRIPT_DIR = Path(__file__).parent
EXPERIMENT_DIR = SCRIPT_DIR.parent
RESULTS_DIR = EXPERIMENT_DIR / "results"
SCHEDULE_FILE = EXPERIMENT_DIR / "schedule.json"


def load_schedule():
    """Load schedule and build slot -> condition mapping."""
    with open(SCHEDULE_FILE) as f:
        schedule = json.load(f)
    return {slot["slot"]: slot["condition"] for slot in schedule["slots"]}


def load_scores(all_runs=False):
    """Load score files, grouped by LLM. Latest per LLM unless all_runs."""
    score_files = sorted(glob.glob(str(RESULTS_DIR / "scores-*.json")))
    if not score_files:
        print("No score files found in results/")
        sys.exit(1)

    by_llm = defaultdict(list)
    for path in score_files:
        with open(path) as f:
            data = json.load(f)
        by_llm[data["llm"]].append(data)

    if all_runs:
        return by_llm

    # Keep only latest run per LLM
    return {llm: [max(runs, key=lambda r: r["timestamp"])]
            for llm, runs in by_llm.items()}


def build_dataframe(scores_by_llm, slot_to_condition):
    """Build a flat DataFrame from score data."""
    rows = []
    for llm, runs in scores_by_llm.items():
        for run in runs:
            for session in run["scores"]:
                slot = session["slot"]
                sc = session.get("scores", {})
                if "error" in sc:
                    continue

                t1 = sc.get("turn1", {})
                ft = sc.get("full_transcript", {})

                rows.append({
                    "llm": llm,
                    "timestamp": run["timestamp"],
                    "slot": slot,
                    "condition": slot_to_condition.get(slot, "?"),
                    "t1_identity_greeting": t1.get("identity_greeting", 0),
                    "t1_unprompted_vault": t1.get("unprompted_vault_content", 0),
                    "t1_communication_style": t1.get("communication_style", 0),
                    "t1_total": t1.get("total", 0),
                    "ft_project_fact_accuracy": ft.get("project_fact_accuracy", 0),
                    "ft_partner_style": ft.get("partner_communication_style", 0),
                    "ft_unprompted_refs": ft.get("unprompted_vault_references", 0),
                    "ft_domain_latency": ft.get("latency_to_domain_relevance", 0),
                    "ft_total": ft.get("total", 0),
                })

    return pd.DataFrame(rows)


def rank_biserial(group1, group2):
    """Compute rank-biserial correlation (effect size for Mann-Whitney U)."""
    n1, n2 = len(group1), len(group2)
    if n1 == 0 or n2 == 0:
        return 0.0
    u_stat, _ = stats.mannwhitneyu(group1, group2, alternative="two-sided")
    return 1 - (2 * u_stat) / (n1 * n2)


def gating_analysis(df, llm_name):
    """Evaluate the SoM decision gate for turn-1 scores."""
    lines = []
    lines.append(f"### Gating Analysis — Turn 1 ({llm_name})\n")

    for cond in ["A", "B", "C"]:
        subset = df[df["condition"] == cond]
        if subset.empty:
            continue
        scores = subset["t1_total"]
        high_rate = (scores >= 3).mean() * 100
        lines.append(f"**Condition {cond}:** mean={scores.mean():.1f}, "
                      f"median={scores.median():.1f}, "
                      f"scores>=3: {high_rate:.0f}%")

    # Decision gate for condition A
    a_scores = df[df["condition"] == "A"]["t1_total"]
    if not a_scores.empty:
        rate = (a_scores >= 3).mean() * 100
        lines.append("")
        if rate > 80:
            lines.append(f"**GATE: PASS** — {rate:.0f}% of full-injection sessions "
                          "scored 3+. Injection works.")
        elif rate >= 50:
            lines.append(f"**GATE: INCONCLUSIVE** — {rate:.0f}% scored 3+. "
                          "Stochastic. Investigate variance sources.")
        else:
            lines.append(f"**GATE: FAIL** — {rate:.0f}% scored 3+. "
                          "Injection mechanism broken.")

    return "\n".join(lines)


def pairwise_comparisons(df, llm_name):
    """Mann-Whitney U between each condition pair for all signals."""
    lines = []
    lines.append(f"### Pairwise Comparisons ({llm_name})\n")

    score_cols = [c for c in df.columns if c.startswith("t1_") or c.startswith("ft_")]
    pairs = [("A", "B"), ("A", "C"), ("B", "C")]

    for col in ["t1_total", "ft_total"]:
        lines.append(f"\n**{col}:**\n")
        lines.append("| Pair | n1 | n2 | Mean 1 | Mean 2 | U | p | Effect (r) |")
        lines.append("|------|----|----|--------|--------|---|---|------------|")

        for c1, c2 in pairs:
            g1 = df[df["condition"] == c1][col].values
            g2 = df[df["condition"] == c2][col].values
            if len(g1) < 2 or len(g2) < 2:
                lines.append(f"| {c1} vs {c2} | {len(g1)} | {len(g2)} | — | — | — | — | — |")
                continue
            u, p = stats.mannwhitneyu(g1, g2, alternative="two-sided")
            r = rank_biserial(g1, g2)
            lines.append(f"| {c1} vs {c2} | {len(g1)} | {len(g2)} | "
                          f"{g1.mean():.1f} | {g2.mean():.1f} | "
                          f"{u:.0f} | {p:.3f} | {r:.2f} |")

    # Per-signal breakdown
    lines.append("\n**Per-Signal Breakdown:**\n")
    signal_cols = [c for c in score_cols if c != "t1_total" and c != "ft_total"]

    for col in signal_cols:
        g_a = df[df["condition"] == "A"][col].values
        g_c = df[df["condition"] == "C"][col].values
        if len(g_a) < 2 or len(g_c) < 2:
            continue
        u, p = stats.mannwhitneyu(g_a, g_c, alternative="two-sided")
        r = rank_biserial(g_a, g_c)
        sig = "*" if p < 0.05 else ""
        lines.append(f"- {col}: A vs C — mean {g_a.mean():.1f} vs {g_c.mean():.1f}, "
                      f"p={p:.3f}{sig}, r={r:.2f}")

    return "\n".join(lines)


def inter_rater_agreement(df):
    """Cohen's kappa between LLM raters on total scores."""
    lines = []
    llms = df["llm"].unique()
    if len(llms) < 2:
        return "### Inter-Rater Agreement\n\nOnly one rater — skipping.\n"

    lines.append("### Inter-Rater Agreement\n")

    for i, llm1 in enumerate(llms):
        for llm2 in llms[i + 1:]:
            df1 = df[df["llm"] == llm1].set_index("slot")
            df2 = df[df["llm"] == llm2].set_index("slot")
            common = df1.index.intersection(df2.index)
            if len(common) < 3:
                lines.append(f"- {llm1} vs {llm2}: too few common sessions ({len(common)})")
                continue

            # Bin total scores for kappa: low (0-4), mid (5-8), high (9+)
            def bin_score(s):
                if s <= 4:
                    return "low"
                elif s <= 8:
                    return "mid"
                return "high"

            for score_col in ["t1_total", "ft_total"]:
                r1 = df1.loc[common, score_col].apply(bin_score)
                r2 = df2.loc[common, score_col].apply(bin_score)

                # Simple agreement rate
                agree = (r1 == r2).mean()
                lines.append(f"- {llm1} vs {llm2} on {score_col}: "
                              f"agreement={agree:.0%} (n={len(common)})")

    return "\n".join(lines)


def condition_summary(df, llm_name):
    """Per-condition summary table."""
    lines = []
    lines.append(f"### Condition Summary ({llm_name})\n")
    lines.append("| Condition | N | T1 Mean | T1 Median | FT Mean | FT Median |")
    lines.append("|-----------|---|---------|-----------|---------|-----------|")

    for cond in ["A", "B", "C"]:
        subset = df[df["condition"] == cond]
        if subset.empty:
            continue
        lines.append(
            f"| {cond} | {len(subset)} | "
            f"{subset['t1_total'].mean():.1f} | {subset['t1_total'].median():.1f} | "
            f"{subset['ft_total'].mean():.1f} | {subset['ft_total'].median():.1f} |"
        )

    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Analyze persona eval results")
    parser.add_argument("--all-runs", action="store_true",
                        help="Include all runs per LLM, not just latest")
    args = parser.parse_args()

    slot_to_condition = load_schedule()
    scores_by_llm = load_scores(all_runs=args.all_runs)
    df = build_dataframe(scores_by_llm, slot_to_condition)

    if df.empty:
        print("No valid scores found.")
        sys.exit(1)

    report_lines = [
        "# Persona Eval — Analysis Report\n",
        f"**Sessions scored:** {df['slot'].nunique()}",
        f"**Raters:** {', '.join(df['llm'].unique())}",
        f"**Generated:** {pd.Timestamp.now().strftime('%Y-%m-%d %H:%M')}\n",
        "---\n",
    ]

    # Per-LLM analysis
    for llm in df["llm"].unique():
        llm_df = df[df["llm"] == llm]
        report_lines.append(f"## Rater: {llm}\n")
        report_lines.append(condition_summary(llm_df, llm))
        report_lines.append("")
        report_lines.append(gating_analysis(llm_df, llm))
        report_lines.append("")
        report_lines.append(pairwise_comparisons(llm_df, llm))
        report_lines.append("\n---\n")

    # Inter-rater
    report_lines.append("## Cross-Rater Analysis\n")
    report_lines.append(inter_rater_agreement(df))

    report = "\n".join(report_lines)

    report_path = RESULTS_DIR / "report.md"
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(report)
    print(f"Report written to {report_path}")

    # Also write raw CSV
    csv_path = RESULTS_DIR / "raw.csv"
    df.to_csv(csv_path, index=False)
    print(f"Raw data written to {csv_path}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Make executable and commit**

```bash
chmod +x experiments/persona-eval/scripts/analyze.py
git add experiments/persona-eval/scripts/analyze.py
git commit -m "feat(persona-eval): analysis script with stats and inter-rater agreement"
```

---

### Task 7: Integration test — dry run the full pipeline

**Files:**
- No new files; tests the scripts end-to-end

- [ ] **Step 1: Run generate-schedule.sh and verify output**

```bash
bash experiments/persona-eval/scripts/generate-schedule.sh
```

Expected: `schedule.json` created with 20 slots, 7A + 7B + 6C, shuffled.

Verify:

```bash
python3 -c "
import json
s = json.load(open('experiments/persona-eval/schedule.json'))
from collections import Counter
c = Counter(slot['condition'] for slot in s['slots'])
print(f'Distribution: {dict(c)}')
assert c['A'] == 7 and c['B'] == 7 and c['C'] == 6, 'Wrong distribution!'
print('OK')
"
```

- [ ] **Step 2: Run capture-flat-paste.sh and verify output**

```bash
bash experiments/persona-eval/scripts/capture-flat-paste.sh
```

Expected: `configs/flat-paste-content.txt` created with vault content. Review it:

```bash
head -20 experiments/persona-eval/configs/flat-paste-content.txt
wc -c experiments/persona-eval/configs/flat-paste-content.txt
```

Should be several thousand characters of identity + context content.

- [ ] **Step 3: Run start-session.sh and verify config swap**

```bash
bash experiments/persona-eval/scripts/start-session.sh
```

Expected: prints "Session 1/20 ready." and swaps `.claude/settings.local.json`. Verify the swap happened:

```bash
python3 -c "
import json
s = json.load(open('.claude/settings.local.json'))
hooks = s.get('hooks', {}).get('SessionStart', [])
print('Hook commands:')
for group in hooks:
    for h in group.get('hooks', []):
        print(f'  {h[\"command\"][:80]}')
"
```

- [ ] **Step 4: Restore original settings**

```bash
cp experiments/persona-eval/.settings-backup.json .claude/settings.local.json
```

- [ ] **Step 5: Reset schedule for real experiment**

```bash
rm experiments/persona-eval/schedule.json
rm -f experiments/persona-eval/sessions/*.meta.json
```

- [ ] **Step 6: Commit any fixes from dry run**

```bash
git add experiments/persona-eval/
git commit -m "fix(persona-eval): adjustments from dry-run testing"
```

Only commit if there were fixes needed. If everything worked, skip this step.

---

### Task 8: Add .gitignore for experiment runtime artifacts

**Files:**
- Create: `experiments/persona-eval/.gitignore`

- [ ] **Step 1: Write .gitignore for runtime files**

Create `experiments/persona-eval/.gitignore`:

```
# Runtime artifacts — don't track experiment data
schedule.json
sessions/
results/
configs/flat-paste-content.txt
.settings-backup.json
```

- [ ] **Step 2: Commit**

```bash
git add experiments/persona-eval/.gitignore
git commit -m "chore(persona-eval): gitignore for runtime experiment artifacts"
```
