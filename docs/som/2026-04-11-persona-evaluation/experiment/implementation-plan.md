# Persona Eval Blinded Measurement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the tooling for a 20-session blinded experiment measuring whether VaultMind identity injection produces detectable behavioral differences.

**Architecture:** Shell scripts manage session lifecycle (schedule, config swap, transcript discovery). A shell scorer sends transcripts to configurable LLM APIs. A Python script does statistical analysis. Three static hook config variants define the conditions.

**Tech Stack:** Bash, jq, curl (session management + scoring), Python 3 with numpy/scipy/pandas (analysis)

**Spec:** `docs/som/2026-04-11-persona-evaluation/experiment/protocol-design.md`

**Build philosophy:** Each task produces a runnable, verifiable artifact. Every task ends with a verification step that proves it works before moving on. No task depends on unverified work from a prior task.

---

### Task 1: Directory scaffold and gitignore

**Files:**
- Create: `experiments/persona-eval/.gitignore`
- Create: `experiments/persona-eval/rubric.md`
- Create: `experiments/persona-eval/README.md`

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p experiments/persona-eval/{configs,scripts,sessions,results}
```

- [ ] **Step 2: Write .gitignore for runtime artifacts**

Create `experiments/persona-eval/.gitignore`:

```
# Runtime artifacts — don't track experiment data
schedule.json
sessions/
results/
configs/flat-paste-content.txt
.settings-backup.json
```

- [ ] **Step 3: Write the scoring rubric**

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

- [ ] **Step 4: Write a minimal README**

Create `experiments/persona-eval/README.md`:

```markdown
# Persona Injection Blinded Measurement

Layer 1 experiment: does VaultMind identity injection produce detectable
behavioral differences in Claude Code sessions?

## Quick Start

```bash
# 1. One-time setup
bash experiments/persona-eval/scripts/generate-schedule.sh
bash experiments/persona-eval/scripts/capture-flat-paste.sh

# 2. Before each session (20 times)
bash experiments/persona-eval/scripts/start-session.sh

# 3. After all 20 sessions
bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o

# 4. Analyze
python3 experiments/persona-eval/scripts/analyze.py
```

## Design

See `docs/som/2026-04-11-persona-evaluation/experiment/protocol-design.md`
```

- [ ] **Step 5: Verify and commit**

```bash
ls -R experiments/persona-eval/
# Expect: configs/ scripts/ sessions/ results/ .gitignore rubric.md README.md
cat experiments/persona-eval/.gitignore
# Expect: schedule.json, sessions/, results/, configs/flat-paste-content.txt, .settings-backup.json
git add experiments/persona-eval/
git commit -m "feat(persona-eval): scaffold experiment directory with rubric and README"
```

---

### Task 2: Schedule generator

**Files:**
- Create: `experiments/persona-eval/scripts/generate-schedule.sh`

- [ ] **Step 1: Write generate-schedule.sh**

Create `experiments/persona-eval/scripts/generate-schedule.sh`:

```bash
#!/bin/bash
# Generate a randomized 20-session schedule for the persona eval experiment.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"

if [ -f "$SCHEDULE" ]; then
  echo "ERROR: schedule.json already exists. Delete it first to regenerate."
  exit 1
fi

SEED=$(date +%s)

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
```

- [ ] **Step 2: Make executable**

```bash
chmod +x experiments/persona-eval/scripts/generate-schedule.sh
```

- [ ] **Step 3: Run it and verify**

```bash
bash experiments/persona-eval/scripts/generate-schedule.sh
```

Expected: prints seed and path.

```bash
python3 -c "
import json
s = json.load(open('experiments/persona-eval/schedule.json'))
from collections import Counter
c = Counter(slot['condition'] for slot in s['slots'])
print(f'Total slots: {len(s[\"slots\"])}')
print(f'Distribution: {dict(c)}')
assert len(s['slots']) == 20, f'Expected 20 slots, got {len(s[\"slots\"])}'
assert c['A'] == 7, f'Expected 7 A, got {c[\"A\"]}'
assert c['B'] == 7, f'Expected 7 B, got {c[\"B\"]}'
assert c['C'] == 6, f'Expected 6 C, got {c[\"C\"]}'
assert all(sl['status'] == 'pending' for sl in s['slots']), 'All slots should be pending'
print('ALL CHECKS PASSED')
"
```

- [ ] **Step 4: Verify re-run protection**

```bash
bash experiments/persona-eval/scripts/generate-schedule.sh 2>&1
# Expect: ERROR: schedule.json already exists
```

- [ ] **Step 5: Clean up test output and commit**

```bash
rm experiments/persona-eval/schedule.json
git add experiments/persona-eval/scripts/generate-schedule.sh
git commit -m "feat(persona-eval): schedule generation with seed and verification"
```

---

### Task 3: Capture flat-paste content

**Files:**
- Create: `experiments/persona-eval/scripts/capture-flat-paste.sh`

- [ ] **Step 1: Write capture-flat-paste.sh**

Create `experiments/persona-eval/scripts/capture-flat-paste.sh`:

```bash
#!/bin/bash
# Capture a real vault injection output for use as the flat-paste condition.
# Run once during experiment setup, before session 1.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
OUTPUT="$SCRIPT_DIR/../configs/flat-paste-content.txt"

VAULTMIND="/tmp/vaultmind"
VAULT_PATH="$PROJECT_DIR/vaultmind-identity"

if [ ! -f "$VAULTMIND" ]; then
  echo "Building vaultmind..."
  (cd "$PROJECT_DIR" && go build -o "$VAULTMIND" .)
fi

if [ ! -f "$VAULTMIND" ] || [ ! -d "$VAULT_PATH" ]; then
  echo "ERROR: vaultmind binary or vault not found"
  echo "  Binary: $VAULTMIND (exists: $([ -f "$VAULTMIND" ] && echo yes || echo no))"
  echo "  Vault: $VAULT_PATH (exists: $([ -d "$VAULT_PATH" ] && echo yes || echo no))"
  exit 1
fi

IDENTITY=$("$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000)
CONTEXT=$("$VAULTMIND" ask "what matters most right now" --vault "$VAULT_PATH" --max-items 3 --budget 2000)

if [ -z "$IDENTITY" ]; then
  echo "ERROR: vault returned empty identity"
  exit 1
fi

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
```

- [ ] **Step 2: Make executable**

```bash
chmod +x experiments/persona-eval/scripts/capture-flat-paste.sh
```

- [ ] **Step 3: Run it and verify**

```bash
bash experiments/persona-eval/scripts/capture-flat-paste.sh
```

Expected: prints byte count.

```bash
# Verify content looks like real vault output
head -5 experiments/persona-eval/configs/flat-paste-content.txt
# Expect: "IDENTITY CONTEXT:" followed by vault search results

wc -c < experiments/persona-eval/configs/flat-paste-content.txt
# Expect: several thousand bytes (2000-8000 range)

# Verify it matches production hook output format
grep -c "IDENTITY CONTEXT:" experiments/persona-eval/configs/flat-paste-content.txt
# Expect: 1
grep -c "CURRENT CONTEXT:" experiments/persona-eval/configs/flat-paste-content.txt
# Expect: 1
```

- [ ] **Step 4: Clean up test output and commit**

```bash
rm experiments/persona-eval/configs/flat-paste-content.txt
git add experiments/persona-eval/scripts/capture-flat-paste.sh
git commit -m "feat(persona-eval): flat-paste content capture from live vault"
```

---

### Task 4: Hook config variant — condition A (full injection)

**Files:**
- Create: `experiments/persona-eval/configs/condition-a-full.json`

Build and test ONE config variant before doing the others.

- [ ] **Step 1: Copy current settings.local.json as condition A**

The full injection condition is identical to the current production config.

```bash
cp .claude/settings.local.json experiments/persona-eval/configs/condition-a-full.json
```

- [ ] **Step 2: Verify the copy is valid JSON with the right hooks**

```bash
python3 -c "
import json
c = json.load(open('experiments/persona-eval/configs/condition-a-full.json'))
hooks = c['hooks']['SessionStart'][0]['hooks']
commands = [h['command'] for h in hooks]
assert any('load-persona.sh' in cmd for cmd in commands), 'Missing persona hook'
assert any('install_tools.sh' in cmd for cmd in commands), 'Missing install hook'
assert 'permissions' in c, 'Missing permissions block'
print('Condition A config valid')
for cmd in commands:
    print(f'  Hook: {cmd[:70]}')
"
```

- [ ] **Step 3: Test the config swap round-trip**

```bash
# Backup current config
cp .claude/settings.local.json /tmp/settings-backup-test.json

# Swap in condition A
cp experiments/persona-eval/configs/condition-a-full.json .claude/settings.local.json

# Verify it's now the condition A file
diff .claude/settings.local.json experiments/persona-eval/configs/condition-a-full.json
# Expect: no differences

# Restore original
cp /tmp/settings-backup-test.json .claude/settings.local.json
rm /tmp/settings-backup-test.json
echo "Round-trip swap test passed"
```

- [ ] **Step 4: Commit**

```bash
git add experiments/persona-eval/configs/condition-a-full.json
git commit -m "feat(persona-eval): condition A config (full vault injection)"
```

---

### Task 5: Hook config variant — condition B (flat paste)

**Files:**
- Create: `experiments/persona-eval/scripts/inject-flat-paste.sh`
- Create: `experiments/persona-eval/configs/condition-b-flat.json`

- [ ] **Step 1: Write the flat-paste injection script**

Create `experiments/persona-eval/scripts/inject-flat-paste.sh`:

```bash
#!/bin/bash
# Inject hardcoded flat-paste content for condition B.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FLAT_FILE="$SCRIPT_DIR/../configs/flat-paste-content.txt"

if [ -f "$FLAT_FILE" ]; then
  cat "$FLAT_FILE"
else
  echo "ERROR: flat-paste-content.txt not found. Run capture-flat-paste.sh first." >&2
fi
```

- [ ] **Step 2: Make executable and test the script standalone**

```bash
chmod +x experiments/persona-eval/scripts/inject-flat-paste.sh

# Create a test flat-paste file
echo "IDENTITY CONTEXT:
Test identity content

CURRENT CONTEXT:
Test context content" > experiments/persona-eval/configs/flat-paste-content.txt

# Run it
OUTPUT=$(bash experiments/persona-eval/scripts/inject-flat-paste.sh)
echo "$OUTPUT"
# Expect: the test content printed to stdout

# Verify it contains both headers
echo "$OUTPUT" | grep -q "IDENTITY CONTEXT:" && echo "Identity header: OK" || echo "Identity header: MISSING"
echo "$OUTPUT" | grep -q "CURRENT CONTEXT:" && echo "Context header: OK" || echo "Context header: MISSING"

# Clean up test file
rm experiments/persona-eval/configs/flat-paste-content.txt
```

- [ ] **Step 3: Create condition B config from condition A**

```bash
python3 -c "
import json

with open('experiments/persona-eval/configs/condition-a-full.json') as f:
    config = json.load(f)

# Replace the persona hook with flat-paste hook
for group in config['hooks']['SessionStart']:
    new_hooks = []
    for hook in group['hooks']:
        if 'load-persona.sh' in hook['command']:
            hook = {
                'type': 'command',
                'command': 'bash \"\$CLAUDE_PROJECT_DIR\"/experiments/persona-eval/scripts/inject-flat-paste.sh'
            }
        new_hooks.append(hook)
    group['hooks'] = new_hooks

with open('experiments/persona-eval/configs/condition-b-flat.json', 'w') as f:
    json.dump(config, f, indent=2)

print('Condition B config created')
"
```

- [ ] **Step 4: Verify condition B config**

```bash
python3 -c "
import json
c = json.load(open('experiments/persona-eval/configs/condition-b-flat.json'))
hooks = c['hooks']['SessionStart'][0]['hooks']
commands = [h['command'] for h in hooks]

assert any('inject-flat-paste.sh' in cmd for cmd in commands), 'Missing flat-paste hook'
assert not any('load-persona.sh' in cmd for cmd in commands), 'Should NOT have persona hook'
assert any('install_tools.sh' in cmd for cmd in commands), 'Missing install hook'
assert 'permissions' in c, 'Missing permissions block'

# Verify permissions match condition A
a = json.load(open('experiments/persona-eval/configs/condition-a-full.json'))
assert c['permissions'] == a['permissions'], 'Permissions mismatch with condition A!'
print('Condition B config valid')
for cmd in commands:
    print(f'  Hook: {cmd[:70]}')
"
```

- [ ] **Step 5: Commit**

```bash
git add experiments/persona-eval/scripts/inject-flat-paste.sh experiments/persona-eval/configs/condition-b-flat.json
git commit -m "feat(persona-eval): condition B config (flat paste injection)"
```

---

### Task 6: Hook config variant — condition C (instruction only)

**Files:**
- Create: `experiments/persona-eval/scripts/inject-instruction.sh`
- Create: `experiments/persona-eval/configs/condition-c-instruction.json`

- [ ] **Step 1: Write the instruction injection script**

Create `experiments/persona-eval/scripts/inject-instruction.sh`:

```bash
#!/bin/bash
# Inject minimal instruction for condition C.
echo "You are working with Peiman on VaultMind, a memory system for AI agents. The codebase is a Go CLI project."
```

- [ ] **Step 2: Make executable and test**

```bash
chmod +x experiments/persona-eval/scripts/inject-instruction.sh

OUTPUT=$(bash experiments/persona-eval/scripts/inject-instruction.sh)
echo "$OUTPUT"
# Expect: one line of instruction text

# Verify it does NOT contain vault content or cueing language
echo "$OUTPUT" | grep -qi "partner" && echo "FAIL: contains partner cueing" || echo "OK: no partner cueing"
echo "$OUTPUT" | grep -qi "level 3" && echo "FAIL: contains level 3" || echo "OK: no level 3"
echo "$OUTPUT" | grep -qi "identity" && echo "FAIL: contains identity" || echo "OK: no identity cueing"
echo "$OUTPUT" | grep -qi "arcs" && echo "FAIL: contains arcs" || echo "OK: no arcs"
```

- [ ] **Step 3: Create condition C config from condition A**

```bash
python3 -c "
import json

with open('experiments/persona-eval/configs/condition-a-full.json') as f:
    config = json.load(f)

for group in config['hooks']['SessionStart']:
    new_hooks = []
    for hook in group['hooks']:
        if 'load-persona.sh' in hook['command']:
            hook = {
                'type': 'command',
                'command': 'bash \"\$CLAUDE_PROJECT_DIR\"/experiments/persona-eval/scripts/inject-instruction.sh'
            }
        new_hooks.append(hook)
    group['hooks'] = new_hooks

with open('experiments/persona-eval/configs/condition-c-instruction.json', 'w') as f:
    json.dump(config, f, indent=2)

print('Condition C config created')
"
```

- [ ] **Step 4: Verify condition C config**

```bash
python3 -c "
import json
c = json.load(open('experiments/persona-eval/configs/condition-c-instruction.json'))
hooks = c['hooks']['SessionStart'][0]['hooks']
commands = [h['command'] for h in hooks]

assert any('inject-instruction.sh' in cmd for cmd in commands), 'Missing instruction hook'
assert not any('load-persona.sh' in cmd for cmd in commands), 'Should NOT have persona hook'
assert not any('inject-flat-paste.sh' in cmd for cmd in commands), 'Should NOT have flat-paste hook'
assert any('install_tools.sh' in cmd for cmd in commands), 'Missing install hook'

# Verify permissions match condition A
a = json.load(open('experiments/persona-eval/configs/condition-a-full.json'))
assert c['permissions'] == a['permissions'], 'Permissions mismatch!'
print('Condition C config valid')
for cmd in commands:
    print(f'  Hook: {cmd[:70]}')
"
```

- [ ] **Step 5: Verify ALL three configs are internally consistent**

```bash
python3 -c "
import json

configs = {}
for label, path in [
    ('A', 'experiments/persona-eval/configs/condition-a-full.json'),
    ('B', 'experiments/persona-eval/configs/condition-b-flat.json'),
    ('C', 'experiments/persona-eval/configs/condition-c-instruction.json'),
]:
    configs[label] = json.load(open(path))

# All must have same permissions
assert configs['A']['permissions'] == configs['B']['permissions'] == configs['C']['permissions'], \
    'Permissions mismatch across conditions!'

# All must have PreToolUse hook
for label, c in configs.items():
    assert 'PreToolUse' in c['hooks'], f'Condition {label} missing PreToolUse hook'

# All must have install_tools hook
for label, c in configs.items():
    hooks = c['hooks']['SessionStart'][0]['hooks']
    commands = [h['command'] for h in hooks]
    assert any('install_tools.sh' in cmd for cmd in commands), \
        f'Condition {label} missing install_tools hook'

# Each must have DIFFERENT persona/injection hook
hook_scripts = {}
for label, c in configs.items():
    hooks = c['hooks']['SessionStart'][0]['hooks']
    non_install = [h['command'] for h in hooks if 'install_tools' not in h['command']]
    hook_scripts[label] = non_install[0] if non_install else None

assert 'load-persona.sh' in hook_scripts['A'], 'A should use load-persona.sh'
assert 'inject-flat-paste.sh' in hook_scripts['B'], 'B should use inject-flat-paste.sh'
assert 'inject-instruction.sh' in hook_scripts['C'], 'C should use inject-instruction.sh'

print('ALL THREE CONDITIONS VERIFIED:')
for label, script in hook_scripts.items():
    print(f'  {label}: {script[:60]}')
print('Permissions: consistent across all conditions')
print('PreToolUse: present in all conditions')
print('install_tools: present in all conditions')
"
```

- [ ] **Step 6: Commit**

```bash
git add experiments/persona-eval/scripts/inject-instruction.sh experiments/persona-eval/configs/condition-c-instruction.json
git commit -m "feat(persona-eval): condition C config (instruction only) + cross-condition verification"
```

---

### Task 7: Transcript extractor (standalone, tested before scorer)

**Files:**
- Create: `experiments/persona-eval/scripts/extract-transcript.sh`

The scorer needs to extract readable conversation from JSONL transcripts. Build and test this as a standalone tool FIRST, before building the scorer around it.

- [ ] **Step 1: Write extract-transcript.sh**

Create `experiments/persona-eval/scripts/extract-transcript.sh`:

```bash
#!/bin/bash
# Extract readable USER/ASSISTANT turns from a Claude Code JSONL transcript.
# Usage: extract-transcript.sh <path-to-jsonl>
# Outputs clean text to stdout.
set -e

TRANSCRIPT="$1"

if [ -z "$TRANSCRIPT" ] || [ ! -f "$TRANSCRIPT" ]; then
  echo "Usage: extract-transcript.sh <path-to-jsonl>" >&2
  exit 1
fi

python3 -c "
import json, sys

turns = []
with open('$TRANSCRIPT') as f:
    for line in f:
        try:
            d = json.loads(line)
        except json.JSONDecodeError:
            continue

        t = d.get('type', '')

        # Extract user messages (skip system-generated ones)
        if t == 'user' and isinstance(d.get('message'), dict):
            content = d['message'].get('content', '')
            if isinstance(content, str) and not content.startswith('<'):
                turns.append(f'USER: {content}')

        # Extract assistant text blocks
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

# Deduplicate consecutive identical turns (streaming artifacts)
deduped = []
for turn in turns:
    if not deduped or turn != deduped[-1]:
        deduped.append(turn)

print('\n\n'.join(deduped))
"
```

- [ ] **Step 2: Make executable**

```bash
chmod +x experiments/persona-eval/scripts/extract-transcript.sh
```

- [ ] **Step 3: Test with a real transcript from this project**

```bash
# Find the most recent transcript
LATEST=$(ls -t ~/.claude/projects/-Users-peiman-dev-cli-vaultmind/*.jsonl | head -1)
echo "Testing with: $LATEST"

# Run extractor
bash experiments/persona-eval/scripts/extract-transcript.sh "$LATEST" > /tmp/test-extract.txt

# Verify output
echo "--- First 20 lines ---"
head -20 /tmp/test-extract.txt

echo ""
echo "--- Stats ---"
echo "Total lines: $(wc -l < /tmp/test-extract.txt)"
echo "USER turns: $(grep -c '^USER:' /tmp/test-extract.txt)"
echo "ASSISTANT turns: $(grep -c '^ASSISTANT:' /tmp/test-extract.txt)"

# Verify it starts with a user message
FIRST_TURN=$(grep -m1 '^USER:\|^ASSISTANT:' /tmp/test-extract.txt)
echo "First turn: ${FIRST_TURN:0:60}"

rm /tmp/test-extract.txt
```

Expected: readable conversation with USER and ASSISTANT turns. No raw JSON, no tool calls, no system reminders.

- [ ] **Step 4: Test with a second transcript to catch edge cases**

```bash
SECOND=$(ls -t ~/.claude/projects/-Users-peiman-dev-cli-vaultmind/*.jsonl | head -2 | tail -1)
echo "Testing with: $SECOND"

OUTPUT=$(bash experiments/persona-eval/scripts/extract-transcript.sh "$SECOND")
USERS=$(echo "$OUTPUT" | grep -c '^USER:')
ASSISTANTS=$(echo "$OUTPUT" | grep -c '^ASSISTANT:')
echo "USER turns: $USERS, ASSISTANT turns: $ASSISTANTS"

# Both counts should be > 0
[ "$USERS" -gt 0 ] && [ "$ASSISTANTS" -gt 0 ] && echo "PASS" || echo "FAIL: missing turns"
```

- [ ] **Step 5: Test error handling**

```bash
# Non-existent file
bash experiments/persona-eval/scripts/extract-transcript.sh /tmp/nonexistent.jsonl 2>&1
# Expect: Usage error, exit code 1

# No arguments
bash experiments/persona-eval/scripts/extract-transcript.sh 2>&1
# Expect: Usage error, exit code 1
```

- [ ] **Step 6: Commit**

```bash
git add experiments/persona-eval/scripts/extract-transcript.sh
git commit -m "feat(persona-eval): standalone transcript extractor, tested with real transcripts"
```

---

### Task 8: Start-session script — basic config swap only

**Files:**
- Create: `experiments/persona-eval/scripts/start-session.sh`

Build the simplest version first: read schedule, swap config, print session number. NO auto-completion logic yet.

- [ ] **Step 1: Write start-session.sh (basic version)**

Create `experiments/persona-eval/scripts/start-session.sh`:

```bash
#!/bin/bash
# Start the next experiment session.
# Reads schedule, swaps hook config, prints session number.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXPERIMENT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROJECT_DIR="$(cd "$EXPERIMENT_DIR/../.." && pwd)"
SCHEDULE="$EXPERIMENT_DIR/schedule.json"
SESSIONS_DIR="$EXPERIMENT_DIR/sessions"
SETTINGS_FILE="$PROJECT_DIR/.claude/settings.local.json"
BACKUP_FILE="$EXPERIMENT_DIR/.settings-backup.json"

if [ ! -f "$SCHEDULE" ]; then
  echo "ERROR: No schedule.json found. Run generate-schedule.sh first."
  exit 1
fi

mkdir -p "$SESSIONS_DIR"

# --- Find next pending slot ---
NEXT_INFO=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['status'] == 'pending':
        print(f\"{slot['slot']}|{slot['condition']}\")
        break
else:
    print('DONE')
")

if [ "$NEXT_INFO" = "DONE" ]; then
  COMPLETED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
print(len([sl for sl in s['slots'] if sl['status'] == 'complete']))
")
  echo "All sessions complete! ($COMPLETED done)"
  echo "Run: bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o"
  # Restore original settings
  if [ -f "$BACKUP_FILE" ]; then
    cp "$BACKUP_FILE" "$SETTINGS_FILE"
    echo "Original settings.local.json restored."
  fi
  exit 0
fi

NEXT_SLOT="${NEXT_INFO%%|*}"
CONDITION="${NEXT_INFO##*|}"

COMPLETED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
print(len([sl for sl in s['slots'] if sl['status'] == 'complete']))
")
SESSION_NUM=$((COMPLETED + 1))

# --- Backup current settings (once) ---
if [ ! -f "$BACKUP_FILE" ]; then
  cp "$SETTINGS_FILE" "$BACKUP_FILE"
fi

# --- Swap in condition config ---
case "$CONDITION" in
  A) CONFIG="$EXPERIMENT_DIR/configs/condition-a-full.json" ;;
  B) CONFIG="$EXPERIMENT_DIR/configs/condition-b-flat.json" ;;
  C) CONFIG="$EXPERIMENT_DIR/configs/condition-c-instruction.json" ;;
  *) echo "ERROR: Unknown condition $CONDITION"; exit 1 ;;
esac

if [ ! -f "$CONFIG" ]; then
  echo "ERROR: Config file not found: $CONFIG"
  exit 1
fi

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
echo "Session $SESSION_NUM/20 ready. Open a new Claude Code session in this project."
echo ""
```

- [ ] **Step 2: Make executable**

```bash
chmod +x experiments/persona-eval/scripts/start-session.sh
```

- [ ] **Step 3: Test with a real schedule**

```bash
# Generate a test schedule
bash experiments/persona-eval/scripts/generate-schedule.sh

# Run start-session
bash experiments/persona-eval/scripts/start-session.sh
# Expect: "Session 1/20 ready."

# Verify config was swapped
python3 -c "
import json
c = json.load(open('.claude/settings.local.json'))
hooks = c['hooks']['SessionStart'][0]['hooks']
commands = [h['command'] for h in hooks]
print('Current hooks:')
for cmd in commands:
    print(f'  {cmd[:70]}')
"

# Verify schedule was updated
python3 -c "
import json
s = json.load(open('experiments/persona-eval/schedule.json'))
started = [sl for sl in s['slots'] if sl['status'] == 'started']
print(f'Started slots: {len(started)}')
assert len(started) == 1, 'Expected exactly 1 started slot'
print(f'Slot {started[0][\"slot\"]}: condition {started[0][\"condition\"]}')
print('PASS')
"

# Verify meta file was created
ls experiments/persona-eval/sessions/
cat experiments/persona-eval/sessions/session-*.meta.json
```

- [ ] **Step 4: Restore and clean up**

```bash
# Restore settings
cp experiments/persona-eval/.settings-backup.json .claude/settings.local.json

# Clean up test artifacts
rm experiments/persona-eval/schedule.json
rm -f experiments/persona-eval/sessions/*.meta.json
rm -f experiments/persona-eval/.settings-backup.json
```

- [ ] **Step 5: Commit**

```bash
git add experiments/persona-eval/scripts/start-session.sh
git commit -m "feat(persona-eval): basic session start with config swap and schedule tracking"
```

---

### Task 9: Start-session — add auto-completion detection

**Files:**
- Modify: `experiments/persona-eval/scripts/start-session.sh`

Now add the logic that detects whether the previous session produced a transcript and auto-completes it.

- [ ] **Step 1: Add auto-completion logic before the "find next pending" block**

In `experiments/persona-eval/scripts/start-session.sh`, add this block after the `mkdir -p "$SESSIONS_DIR"` line and before the `# --- Find next pending slot ---` comment:

```bash
TRANSCRIPT_DIR="$HOME/.claude/projects/-Users-peiman-dev-cli-vaultmind"

# --- Auto-complete previous session if transcript exists ---
LAST_STARTED=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
started = [sl for sl in s['slots'] if sl['status'] == 'started']
if started:
    print(started[0]['slot'])
else:
    print('')
")

if [ -n "$LAST_STARTED" ]; then
  META_FILE="$SESSIONS_DIR/session-$(printf '%02d' "$LAST_STARTED").meta.json"
  STARTED_AT=$(python3 -c "
import json
m = json.load(open('$META_FILE'))
print(m['started_at'])
")

  # Find most recent transcript created after session started
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
    echo "WARNING: Session $LAST_STARTED started but no transcript found. Re-using slot."
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
```

- [ ] **Step 2: Test auto-completion with a real transcript**

```bash
# Setup: generate schedule and start a session
bash experiments/persona-eval/scripts/generate-schedule.sh
bash experiments/persona-eval/scripts/start-session.sh
# This starts session 1 (status: started)

# Simulate a completed session — touch a transcript file newer than start time
LATEST_REAL=$(ls -t ~/.claude/projects/-Users-peiman-dev-cli-vaultmind/*.jsonl | head -1)
echo "Latest real transcript: $LATEST_REAL"

# Now run start-session again — it should auto-complete session 1 and start session 2
bash experiments/persona-eval/scripts/start-session.sh

# Verify session 1 is complete and session 2 is started
python3 -c "
import json
s = json.load(open('experiments/persona-eval/schedule.json'))
for sl in s['slots'][:3]:
    print(f\"  Slot {sl['slot']}: {sl['status']}\")
complete = [sl for sl in s['slots'] if sl['status'] == 'complete']
started = [sl for sl in s['slots'] if sl['status'] == 'started']
print(f'Complete: {len(complete)}, Started: {len(started)}')
"
```

- [ ] **Step 3: Test re-use of abandoned slot (no transcript)**

```bash
# Reset schedule with a slot in 'started' state but faked future timestamp
python3 -c "
import json, time
s = json.load(open('experiments/persona-eval/schedule.json'))
# Set all to pending, then mark slot 1 as started with a future timestamp
for sl in s['slots']:
    sl['status'] = 'pending'
    sl['started_at'] = None
    sl['transcript_path'] = None
s['slots'][0]['status'] = 'started'
s['slots'][0]['started_at'] = str(time.time() + 9999)  # future = no transcript will match
with open('experiments/persona-eval/schedule.json', 'w') as f:
    json.dump(s, f, indent=2)
"

# Create a matching meta file
python3 -c "
import json, time
meta = {'slot': 1, 'condition': 'A', 'status': 'started', 'started_at': str(time.time() + 9999), 'transcript_path': None}
with open('experiments/persona-eval/sessions/session-01.meta.json', 'w') as f:
    json.dump(meta, f, indent=2)
"

bash experiments/persona-eval/scripts/start-session.sh 2>&1
# Expect: "WARNING: Session 1 started but no transcript found. Re-using slot."
# Then: "Session 1/20 ready."
```

- [ ] **Step 4: Clean up and restore**

```bash
cp experiments/persona-eval/.settings-backup.json .claude/settings.local.json
rm experiments/persona-eval/schedule.json
rm -f experiments/persona-eval/sessions/*.meta.json
rm -f experiments/persona-eval/.settings-backup.json
```

- [ ] **Step 5: Commit**

```bash
git add experiments/persona-eval/scripts/start-session.sh
git commit -m "feat(persona-eval): auto-completion detection for previous sessions"
```

---

### Task 10: Scorer — single provider (OpenAI) only

**Files:**
- Create: `experiments/persona-eval/scripts/score-transcripts.sh`

Build the scorer with ONE provider first. Get it working end-to-end before adding Anthropic and Google.

- [ ] **Step 1: Write score-transcripts.sh with OpenAI support only**

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
EXTRACT_SCRIPT="$SCRIPT_DIR/extract-transcript.sh"

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
  echo "Supported: openai/<model>"
  exit 1
fi

PROVIDER="${LLM%%/*}"
MODEL="${LLM#*/}"
MODEL_SLUG=$(echo "$MODEL" | tr './' '--')
TIMESTAMP=$(date +%Y%m%dT%H%M%S)
OUTPUT_FILE="$RESULTS_DIR/scores-${MODEL_SLUG}-${TIMESTAMP}.json"

mkdir -p "$RESULTS_DIR"

# --- Validate ---
if [ ! -f "$SCHEDULE" ]; then
  echo "ERROR: No schedule.json found."
  exit 1
fi
if [ ! -f "$RUBRIC" ]; then
  echo "ERROR: No rubric.md found."
  exit 1
fi
if [ ! -x "$EXTRACT_SCRIPT" ]; then
  echo "ERROR: extract-transcript.sh not found or not executable."
  exit 1
fi

# --- API setup ---
case "$PROVIDER" in
  openai)
    API_KEY="${OPENAI_API_KEY:?Set OPENAI_API_KEY}"
    API_URL="https://api.openai.com/v1/chat/completions"
    ;;
  *)
    echo "Unsupported provider: $PROVIDER (supported: openai)"
    exit 1
    ;;
esac

# --- Temp directory for payloads ---
TMPDIR_SCORE=$(mktemp -d)
trap "rm -rf $TMPDIR_SCORE" EXIT

SYSTEM_PROMPT="You are a behavioral scoring rater. Score the following Claude Code session transcript using the provided rubric. Return ONLY valid JSON matching the schema in the rubric. Be strict. Quote specific text as evidence."

# --- Score one transcript via OpenAI ---
score_openai() {
  local rubric_file="$1"
  local conversation_file="$2"
  local payload_file="$TMPDIR_SCORE/payload.json"
  local response_file="$TMPDIR_SCORE/response.json"

  python3 -c "
import json

with open('$rubric_file') as f:
    rubric = f.read()
with open('$conversation_file') as f:
    conversation = f.read()

user_content = rubric + '\n\n---\n\n## Transcript to Score\n\n' + conversation

payload = {
    'model': '$MODEL',
    'messages': [
        {'role': 'system', 'content': '''$SYSTEM_PROMPT'''},
        {'role': 'user', 'content': user_content}
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

  # Extract content, handling errors
  python3 -c "
import json, sys
with open('$response_file') as f:
    r = json.load(f)
if 'error' in r:
    print(json.dumps({'error': r['error']['message']}))
elif 'choices' in r:
    print(r['choices'][0]['message']['content'])
else:
    print(json.dumps({'error': 'unexpected response format'}))
"
}

# --- Main scoring loop ---
echo "Scoring transcripts with $LLM..."

RESULTS_FILE="$TMPDIR_SCORE/results.json"
echo "[]" > "$RESULTS_FILE"

COMPLETED_SLOTS=$(python3 -c "
import json
s = json.load(open('$SCHEDULE'))
for slot in s['slots']:
    if slot['status'] == 'complete' and slot.get('transcript_path'):
        print(f\"{slot['slot']}|{slot['transcript_path']}\")
")

if [ -z "$COMPLETED_SLOTS" ]; then
  echo "No completed sessions to score."
  exit 0
fi

TOTAL=$(echo "$COMPLETED_SLOTS" | wc -l | tr -d ' ')
SCORED=0

while IFS='|' read -r SLOT TRANSCRIPT_PATH; do
  [ -z "$SLOT" ] && continue
  SCORED=$((SCORED + 1))
  echo "  Scoring session $SLOT ($SCORED/$TOTAL)..."

  # Extract conversation to temp file
  CONV_FILE="$TMPDIR_SCORE/conversation.txt"
  bash "$EXTRACT_SCRIPT" "$TRANSCRIPT_PATH" > "$CONV_FILE"

  # Score
  SCORE_FILE="$TMPDIR_SCORE/score.json"
  score_openai "$RUBRIC" "$CONV_FILE" > "$SCORE_FILE"

  # Append to results
  python3 -c "
import json
with open('$RESULTS_FILE') as f:
    results = json.load(f)
try:
    with open('$SCORE_FILE') as f:
        raw = f.read().strip()
        score = json.loads(raw)
except (json.JSONDecodeError, Exception) as e:
    score = {'error': f'Failed to parse LLM response: {str(e)}', 'raw': raw[:500] if 'raw' in dir() else ''}
results.append({'slot': $SLOT, 'scores': score})
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

- [ ] **Step 2: Make executable**

```bash
chmod +x experiments/persona-eval/scripts/score-transcripts.sh
```

- [ ] **Step 3: Test argument parsing and validation**

```bash
# No args
bash experiments/persona-eval/scripts/score-transcripts.sh 2>&1
# Expect: Usage message

# Missing schedule
bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o 2>&1
# Expect: ERROR about missing schedule.json

# Unsupported provider
bash experiments/persona-eval/scripts/generate-schedule.sh
bash experiments/persona-eval/scripts/score-transcripts.sh --llm google/gemini 2>&1
# Expect: "Unsupported provider: google"
```

- [ ] **Step 4: Test with a real transcript (requires OPENAI_API_KEY)**

If the API key is available, do a single-transcript test:

```bash
# Create a minimal schedule with one "complete" session pointing to a real transcript
LATEST=$(ls -t ~/.claude/projects/-Users-peiman-dev-cli-vaultmind/*.jsonl | head -1)

python3 -c "
import json
s = json.load(open('experiments/persona-eval/schedule.json'))
s['slots'][0]['status'] = 'complete'
s['slots'][0]['transcript_path'] = '$LATEST'
with open('experiments/persona-eval/schedule.json', 'w') as f:
    json.dump(s, f, indent=2)
"

bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o

# Check output
ls experiments/persona-eval/results/scores-*.json
cat experiments/persona-eval/results/scores-*.json | python3 -m json.tool | head -30
```

Verify the output JSON has the expected structure: `turn1` and `full_transcript` with numeric scores and evidence arrays.

- [ ] **Step 5: Clean up and commit**

```bash
rm experiments/persona-eval/schedule.json
rm -f experiments/persona-eval/results/*.json
git add experiments/persona-eval/scripts/score-transcripts.sh
git commit -m "feat(persona-eval): transcript scorer with OpenAI support"
```

---

### Task 11: Scorer — add Anthropic and Google providers

**Files:**
- Modify: `experiments/persona-eval/scripts/score-transcripts.sh`

Now that OpenAI works, add the other two providers using the same pattern.

- [ ] **Step 1: Add Anthropic API support**

In `score-transcripts.sh`, update the API setup case statement:

```bash
  anthropic)
    API_KEY="${ANTHROPIC_API_KEY:?Set ANTHROPIC_API_KEY}"
    API_URL="https://api.anthropic.com/v1/messages"
    ;;
```

Add the `score_anthropic` function after `score_openai`:

```bash
score_anthropic() {
  local rubric_file="$1"
  local conversation_file="$2"
  local payload_file="$TMPDIR_SCORE/payload.json"
  local response_file="$TMPDIR_SCORE/response.json"

  python3 -c "
import json

with open('$rubric_file') as f:
    rubric = f.read()
with open('$conversation_file') as f:
    conversation = f.read()

user_content = rubric + '\n\n---\n\n## Transcript to Score\n\n' + conversation

payload = {
    'model': '$MODEL',
    'max_tokens': 4096,
    'system': '''$SYSTEM_PROMPT''',
    'messages': [
        {'role': 'user', 'content': user_content}
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
with open('$response_file') as f:
    r = json.load(f)
if 'error' in r:
    print(json.dumps({'error': r['error']['message']}))
elif 'content' in r:
    print(r['content'][0]['text'])
else:
    print(json.dumps({'error': 'unexpected response format'}))
"
}
```

- [ ] **Step 2: Add Google API support**

Add to the case statement:

```bash
  google)
    API_KEY="${GOOGLE_API_KEY:?Set GOOGLE_API_KEY}"
    API_URL="https://generativelanguage.googleapis.com/v1beta/models/${MODEL}:generateContent?key=${API_KEY}"
    ;;
```

Add the `score_google` function:

```bash
score_google() {
  local rubric_file="$1"
  local conversation_file="$2"
  local payload_file="$TMPDIR_SCORE/payload.json"
  local response_file="$TMPDIR_SCORE/response.json"

  python3 -c "
import json

with open('$rubric_file') as f:
    rubric = f.read()
with open('$conversation_file') as f:
    conversation = f.read()

combined = '''$SYSTEM_PROMPT''' + '\n\n' + rubric + '\n\n---\n\n## Transcript to Score\n\n' + conversation

payload = {
    'contents': [{'parts': [{'text': combined}]}],
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
with open('$response_file') as f:
    r = json.load(f)
if 'error' in r:
    print(json.dumps({'error': r['error']['message']}))
elif 'candidates' in r:
    print(r['candidates'][0]['content']['parts'][0]['text'])
else:
    print(json.dumps({'error': 'unexpected response format'}))
"
}
```

- [ ] **Step 3: Update the scoring loop to dispatch by provider**

Replace the direct `score_openai` call in the main loop with:

```bash
  case "$PROVIDER" in
    openai) score_openai "$RUBRIC" "$CONV_FILE" > "$SCORE_FILE" ;;
    anthropic) score_anthropic "$RUBRIC" "$CONV_FILE" > "$SCORE_FILE" ;;
    google) score_google "$RUBRIC" "$CONV_FILE" > "$SCORE_FILE" ;;
  esac
```

Update the usage message:

```bash
  echo "Supported: openai/<model>, anthropic/<model>, google/<model>"
```

- [ ] **Step 4: Verify each provider's error handling without real API keys**

```bash
# Test each provider reports missing key clearly
OPENAI_API_KEY="" bash experiments/persona-eval/scripts/score-transcripts.sh --llm openai/gpt-4o 2>&1 | head -2
# Expect: error about OPENAI_API_KEY

ANTHROPIC_API_KEY="" bash experiments/persona-eval/scripts/score-transcripts.sh --llm anthropic/claude-sonnet-4-6 2>&1 | head -2
# Expect: error about ANTHROPIC_API_KEY

GOOGLE_API_KEY="" bash experiments/persona-eval/scripts/score-transcripts.sh --llm google/gemini-2.0-flash 2>&1 | head -2
# Expect: error about GOOGLE_API_KEY
```

- [ ] **Step 5: Commit**

```bash
git add experiments/persona-eval/scripts/score-transcripts.sh
git commit -m "feat(persona-eval): add Anthropic and Google provider support to scorer"
```

---

### Task 12: Analysis script

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
    with open(SCHEDULE_FILE) as f:
        schedule = json.load(f)
    return {slot["slot"]: slot["condition"] for slot in schedule["slots"]}


def load_scores(all_runs=False):
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

    return {llm: [max(runs, key=lambda r: r["timestamp"])]
            for llm, runs in by_llm.items()}


def build_dataframe(scores_by_llm, slot_to_condition):
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
    n1, n2 = len(group1), len(group2)
    if n1 == 0 or n2 == 0:
        return 0.0
    u_stat, _ = stats.mannwhitneyu(group1, group2, alternative="two-sided")
    return 1 - (2 * u_stat) / (n1 * n2)


def gating_analysis(df, llm_name):
    lines = [f"### Gating Analysis -- Turn 1 ({llm_name})\n"]

    for cond in ["A", "B", "C"]:
        subset = df[df["condition"] == cond]
        if subset.empty:
            continue
        scores = subset["t1_total"]
        high_rate = (scores >= 3).mean() * 100
        lines.append(f"**Condition {cond}:** mean={scores.mean():.1f}, "
                      f"median={scores.median():.1f}, "
                      f"scores>=3: {high_rate:.0f}%")

    a_scores = df[df["condition"] == "A"]["t1_total"]
    if not a_scores.empty:
        rate = (a_scores >= 3).mean() * 100
        lines.append("")
        if rate > 80:
            lines.append(f"**GATE: PASS** -- {rate:.0f}% of full-injection sessions "
                          "scored 3+. Injection works.")
        elif rate >= 50:
            lines.append(f"**GATE: INCONCLUSIVE** -- {rate:.0f}% scored 3+. "
                          "Stochastic. Investigate variance sources.")
        else:
            lines.append(f"**GATE: FAIL** -- {rate:.0f}% scored 3+. "
                          "Injection mechanism broken.")

    return "\n".join(lines)


def pairwise_comparisons(df, llm_name):
    lines = [f"### Pairwise Comparisons ({llm_name})\n"]
    pairs = [("A", "B"), ("A", "C"), ("B", "C")]

    for col in ["t1_total", "ft_total"]:
        lines.append(f"\n**{col}:**\n")
        lines.append("| Pair | n1 | n2 | Mean 1 | Mean 2 | U | p | Effect (r) |")
        lines.append("|------|----|----|--------|--------|---|---|------------|")

        for c1, c2 in pairs:
            g1 = df[df["condition"] == c1][col].values
            g2 = df[df["condition"] == c2][col].values
            if len(g1) < 2 or len(g2) < 2:
                lines.append(f"| {c1} vs {c2} | {len(g1)} | {len(g2)} "
                              "| -- | -- | -- | -- | -- |")
                continue
            u, p = stats.mannwhitneyu(g1, g2, alternative="two-sided")
            r = rank_biserial(g1, g2)
            lines.append(f"| {c1} vs {c2} | {len(g1)} | {len(g2)} | "
                          f"{g1.mean():.1f} | {g2.mean():.1f} | "
                          f"{u:.0f} | {p:.3f} | {r:.2f} |")

    # Per-signal breakdown for A vs C
    lines.append("\n**Per-Signal Breakdown (A vs C):**\n")
    signal_cols = [c for c in df.columns
                   if (c.startswith("t1_") or c.startswith("ft_"))
                   and c not in ("t1_total", "ft_total")]

    for col in signal_cols:
        g_a = df[df["condition"] == "A"][col].values
        g_c = df[df["condition"] == "C"][col].values
        if len(g_a) < 2 or len(g_c) < 2:
            continue
        u, p = stats.mannwhitneyu(g_a, g_c, alternative="two-sided")
        r = rank_biserial(g_a, g_c)
        sig = " *" if p < 0.05 else ""
        lines.append(f"- {col}: mean {g_a.mean():.1f} vs {g_c.mean():.1f}, "
                      f"p={p:.3f}{sig}, r={r:.2f}")

    return "\n".join(lines)


def inter_rater_agreement(df):
    llms = df["llm"].unique()
    if len(llms) < 2:
        return "### Inter-Rater Agreement\n\nOnly one rater -- skipping.\n"

    lines = ["### Inter-Rater Agreement\n"]

    for i, llm1 in enumerate(llms):
        for llm2 in llms[i + 1:]:
            df1 = df[df["llm"] == llm1].set_index("slot")
            df2 = df[df["llm"] == llm2].set_index("slot")
            common = df1.index.intersection(df2.index)
            if len(common) < 3:
                lines.append(f"- {llm1} vs {llm2}: too few common sessions "
                              f"({len(common)})")
                continue

            for score_col in ["t1_total", "ft_total"]:
                r1 = df1.loc[common, score_col]
                r2 = df2.loc[common, score_col]
                agree = ((r1 - r2).abs() <= 1).mean()
                lines.append(f"- {llm1} vs {llm2} on {score_col}: "
                              f"within-1-point agreement={agree:.0%} "
                              f"(n={len(common)})")

    return "\n".join(lines)


def condition_summary(df, llm_name):
    lines = [f"### Condition Summary ({llm_name})\n"]
    lines.append("| Condition | N | T1 Mean | T1 Median | FT Mean | FT Median |")
    lines.append("|-----------|---|---------|-----------|---------|-----------|")

    for cond in ["A", "B", "C"]:
        subset = df[df["condition"] == cond]
        if subset.empty:
            continue
        lines.append(
            f"| {cond} | {len(subset)} | "
            f"{subset['t1_total'].mean():.1f} | "
            f"{subset['t1_total'].median():.1f} | "
            f"{subset['ft_total'].mean():.1f} | "
            f"{subset['ft_total'].median():.1f} |"
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
        "# Persona Eval -- Analysis Report\n",
        f"**Sessions scored:** {df['slot'].nunique()}",
        f"**Raters:** {', '.join(df['llm'].unique())}",
        f"**Generated:** {pd.Timestamp.now().strftime('%Y-%m-%d %H:%M')}\n",
        "---\n",
    ]

    for llm in df["llm"].unique():
        llm_df = df[df["llm"] == llm]
        report_lines.append(f"## Rater: {llm}\n")
        report_lines.append(condition_summary(llm_df, llm))
        report_lines.append("")
        report_lines.append(gating_analysis(llm_df, llm))
        report_lines.append("")
        report_lines.append(pairwise_comparisons(llm_df, llm))
        report_lines.append("\n---\n")

    report_lines.append("## Cross-Rater Analysis\n")
    report_lines.append(inter_rater_agreement(df))

    report = "\n".join(report_lines)

    report_path = RESULTS_DIR / "report.md"
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(report)
    print(f"Report written to {report_path}")

    csv_path = RESULTS_DIR / "raw.csv"
    df.to_csv(csv_path, index=False)
    print(f"Raw data written to {csv_path}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Test with synthetic data**

```bash
chmod +x experiments/persona-eval/scripts/analyze.py

# Create synthetic schedule and scores to test the analysis pipeline
mkdir -p experiments/persona-eval/results

python3 -c "
import json, random
random.seed(42)

# Synthetic schedule
schedule = {
    'seed': 42,
    'total_sessions': 6,
    'distribution': {'A': 2, 'B': 2, 'C': 2},
    'condition_labels': {'A': 'full_injection', 'B': 'flat_paste', 'C': 'instruction_only'},
    'slots': [
        {'slot': i, 'condition': c, 'status': 'complete', 'started_at': None, 'transcript_path': None}
        for i, c in enumerate(['A', 'A', 'B', 'B', 'C', 'C'], 1)
    ]
}
with open('experiments/persona-eval/schedule.json', 'w') as f:
    json.dump(schedule, f, indent=2)

# Synthetic scores — condition A scores higher
def make_scores(condition):
    if condition == 'A':
        t1_total = random.choice([3, 4])
        ft_total = random.choice([8, 9, 10])
    elif condition == 'B':
        t1_total = random.choice([2, 3])
        ft_total = random.choice([6, 7, 8])
    else:
        t1_total = random.choice([0, 1])
        ft_total = random.choice([2, 3, 4])
    return {
        'turn1': {'identity_greeting': min(1, t1_total), 'unprompted_vault_content': min(1, max(0, t1_total-1)), 'communication_style': max(0, t1_total-2), 'total': t1_total, 'evidence': ['test']},
        'full_transcript': {'project_fact_accuracy': ft_total//4, 'partner_communication_style': ft_total//4, 'unprompted_vault_references': ft_total//4, 'latency_to_domain_relevance': min(2, ft_total//5), 'total': ft_total, 'evidence': ['test']}
    }

output = {
    'llm': 'openai/gpt-4o',
    'model': 'gpt-4o',
    'timestamp': '20260413T120000',
    'sessions_scored': 6,
    'scores': [{'slot': i, 'scores': make_scores(c)} for i, c in enumerate(['A','A','B','B','C','C'], 1)]
}
with open('experiments/persona-eval/results/scores-gpt-4o-20260413T120000.json', 'w') as f:
    json.dump(output, f, indent=2)

print('Synthetic test data created')
"

# Run analysis
python3 experiments/persona-eval/scripts/analyze.py

# Check outputs exist and are non-empty
echo "--- Report head ---"
head -25 experiments/persona-eval/results/report.md

echo ""
echo "--- CSV head ---"
head -5 experiments/persona-eval/results/raw.csv

echo ""
echo "Report lines: $(wc -l < experiments/persona-eval/results/report.md)"
echo "CSV rows: $(wc -l < experiments/persona-eval/results/raw.csv)"
```

Expected: report.md with condition summaries, gating analysis, pairwise comparisons. CSV with 6 rows of data.

- [ ] **Step 3: Clean up synthetic data and commit**

```bash
rm experiments/persona-eval/schedule.json
rm -rf experiments/persona-eval/results/*
git add experiments/persona-eval/scripts/analyze.py
git commit -m "feat(persona-eval): analysis script with stats, tested with synthetic data"
```

---

### Task 13: End-to-end dry run

**Files:**
- No new files. Tests the complete pipeline.

Run every script in order to verify they work together.

- [ ] **Step 1: Generate schedule**

```bash
bash experiments/persona-eval/scripts/generate-schedule.sh
python3 -c "
import json
s = json.load(open('experiments/persona-eval/schedule.json'))
from collections import Counter
c = Counter(slot['condition'] for slot in s['slots'])
assert c == {'A': 7, 'B': 7, 'C': 6}, f'Bad distribution: {c}'
print(f'Schedule OK: {dict(c)}')
"
```

- [ ] **Step 2: Capture flat paste**

```bash
bash experiments/persona-eval/scripts/capture-flat-paste.sh
[ -s experiments/persona-eval/configs/flat-paste-content.txt ] && echo "Flat paste OK" || echo "FAIL"
```

- [ ] **Step 3: Start session 1**

```bash
bash experiments/persona-eval/scripts/start-session.sh
echo "Settings now:"
python3 -c "
import json
c = json.load(open('.claude/settings.local.json'))
hooks = c['hooks']['SessionStart'][0]['hooks']
for h in hooks:
    cmd = h['command']
    if 'install_tools' not in cmd:
        print(f'  Injection hook: {cmd[:60]}')
"
```

- [ ] **Step 4: Start session 2 (triggers auto-completion of session 1)**

```bash
bash experiments/persona-eval/scripts/start-session.sh
python3 -c "
import json
s = json.load(open('experiments/persona-eval/schedule.json'))
complete = len([sl for sl in s['slots'] if sl['status'] == 'complete'])
started = len([sl for sl in s['slots'] if sl['status'] == 'started'])
print(f'Complete: {complete}, Started: {started}')
assert complete >= 1, 'Session 1 should be complete'
assert started == 1, 'Exactly 1 session should be started'
print('Auto-completion OK')
"
```

- [ ] **Step 5: Test transcript extraction on a real file**

```bash
LATEST=$(ls -t ~/.claude/projects/-Users-peiman-dev-cli-vaultmind/*.jsonl | head -1)
bash experiments/persona-eval/scripts/extract-transcript.sh "$LATEST" | head -10
echo "---"
bash experiments/persona-eval/scripts/extract-transcript.sh "$LATEST" | wc -l
echo "lines extracted"
```

- [ ] **Step 6: Restore settings and clean up ALL test artifacts**

```bash
cp experiments/persona-eval/.settings-backup.json .claude/settings.local.json
rm experiments/persona-eval/schedule.json
rm -f experiments/persona-eval/sessions/*.meta.json
rm -f experiments/persona-eval/.settings-backup.json
rm -f experiments/persona-eval/configs/flat-paste-content.txt
rm -rf experiments/persona-eval/results/*
echo "Cleaned up. Ready for real experiment."
```

- [ ] **Step 7: Commit any fixes discovered during dry run**

```bash
git add experiments/persona-eval/
git status
# Only commit if there are actual changes (fixes found during dry run)
# git commit -m "fix(persona-eval): adjustments from end-to-end dry run"
```
