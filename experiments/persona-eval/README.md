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
