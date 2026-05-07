#!/bin/bash
# auto-rag-evaluate.sh — aggregate auto-rag-guard.sh firings into a
# markdown report for vaultmind feedback.
#
# This is the Manifesto #10 piece: the consumer's hook layer tells the
# producer (vaultmind) what its retrieval looked like in practice —
# which queries returned weak hits, which drifts have no canonical in
# the vault, which patterns repeat across sessions.
#
# Output: /tmp/auto-rag-report-YYYY-MM-DD.md
#
# Sections:
#   1. Catches by drift signature — count, sample commands.
#   2. Vault retrieval quality — for each (drift, query) pair, the
#      consistent top-3 hit_ids and a confidence flag (heuristic: if
#      every firing returns the same top-3 with low scores, the vault
#      lacks a strong canonical).
#   3. Suggested vault improvements — drifts that should have a strong
#      canonical principle/feedback note.
#
# Distributed via `vaultmind hooks install`; the installed copy lives
# at the consumer's `.claude/scripts/auto-rag-evaluate.sh`.
# Run: bash <project>/.claude/scripts/auto-rag-evaluate.sh [--since YYYYMMDD]

set -uo pipefail

LOG_DIR="${HOME}/.vaultmind/auto-rag"
SINCE="${1:-}"
DATE_STAMP=$(date +%Y-%m-%d)
OUT="/tmp/auto-rag-report-${DATE_STAMP}.md"

if [ ! -d "$LOG_DIR" ]; then
  echo "No log dir at $LOG_DIR — auto-rag-guard hasn't fired yet."
  exit 0
fi

# Collect log files. Optional --since YYYYMMDD filter.
FILES=$(ls -1 "$LOG_DIR"/*.json 2>/dev/null || true)
if [ -z "$FILES" ]; then
  echo "No log files in $LOG_DIR."
  exit 0
fi

if [ -n "$SINCE" ]; then
  FILES=$(echo "$FILES" | awk -v since="$SINCE" '
    {
      n = split($0, parts, "/");
      base = parts[n];
      stamp = substr(base, 1, 8);
      if (stamp >= since) print $0;
    }
  ')
fi

if [ -z "$FILES" ]; then
  echo "No firings since $SINCE."
  exit 0
fi

# Aggregate via python for JSON correctness. Pass file list via env
# var (heredoc claims stdin, so can't use that channel).
export AUTO_RAG_FILES="$FILES"
python3 <<'PYEOF' > "$OUT"
import json, os, sys
from collections import defaultdict, Counter
from datetime import date

paths = [p.strip() for p in os.environ.get('AUTO_RAG_FILES', '').splitlines() if p.strip()]
events = []
for p in paths:
    try:
        with open(p) as f:
            events.append(json.load(f))
    except Exception:
        pass

# Group by (drift, query).
groups = defaultdict(list)
for e in events:
    key = (e.get('drift', '?'), e.get('query', '?'))
    groups[key].append(e)

# Drift counts.
drift_counter = Counter(e.get('drift', '?') for e in events)

print(f"# Auto-RAG Hook Report — {date.today().isoformat()}")
print()
print(f"Aggregated **{len(events)}** firings across "
      f"**{len(drift_counter)}** drift signature(s).")
print()
print("Sourced from `~/.vaultmind/auto-rag/*.json`. Each firing is a "
      "PreToolUse hook match — the agent was about to take an action "
      "matching a known auto-mode drift pattern, the hook intercepted "
      "and queried the vault, and the result was injected into the "
      "agent's context via `additionalContext`.")
print()

print("## 1. Catches by drift signature")
print()
for drift, count in drift_counter.most_common():
    print(f"### `{drift}` — {count} firing(s)")
    samples = [e for e in events if e.get('drift') == drift][:3]
    for s in samples:
        target = s.get('target') or s.get('command', '')
        target = target.replace('\n', ' ')
        if len(target) > 120:
            target = target[:117] + '...'
        print(f"- `{target}`  *(at {s.get('timestamp', '?')})*")
    print()

print("## 2. Vault retrieval quality")
print()
print("For each (drift, query) pair, the hit_ids the vault consistently "
      "returns. If the same top hits show up across many firings AND "
      "those hits are not directly addressed at the drift, the vault "
      "may lack a strong canonical for this drift.")
print()
for (drift, query), evs in groups.items():
    hit_seqs = Counter(e.get('hit_ids', '') for e in evs)
    total = len(evs)
    print(f"### `{drift}` — query: `{query}`")
    print(f"Firings: **{total}**")
    print()
    if not hit_seqs:
        print("No hit_ids recorded.")
        print()
        continue
    print("| Top-3 hit_ids | Times returned |")
    print("|---|---|")
    for ids, cnt in hit_seqs.most_common(5):
        if not ids:
            ids = "(empty — vault unreachable or no hits)"
        print(f"| `{ids}` | {cnt}/{total} |")
    print()

print("## 3. Suggested vault improvements")
print()
print("Drifts where the vault returned consistent low-relevance hits "
      "should get a dedicated principle or feedback note in the "
      "consumer's vault — typically under `principles/` or `feedbacks/`. "
      "The auto-RAG hook is only as good as the vault's canonical for "
      "each drift.")
print()
print("**Heuristic:** if a drift's top-3 hit_ids are identical across "
      "all firings AND the hit titles don't directly name the drift "
      "behavior, port the relevant guidance into the vault as a "
      "dedicated note. A `feedback_*` memory in the agent's auto-memory "
      "area should be mirrored into the vault as a principle so "
      "vaultmind retrieval surfaces it as the top hit on the canonical "
      "query.")
print()
for (drift, query), evs in groups.items():
    hit_seqs = Counter(e.get('hit_ids', '') for e in evs)
    total = len(evs)
    if total < 2:
        continue
    top_seq, top_cnt = hit_seqs.most_common(1)[0]
    if top_cnt == total and top_seq:
        print(f"- **`{drift}`**: every firing returned `{top_seq}`. "
              f"Consider porting the canonical for `\"{query}\"` "
              f"directly into the project's vault.")
print()
print("---")
print()
print("Generated by `auto-rag-evaluate.sh` (distributed via `vaultmind hooks install`).")
PYEOF

echo "Report written to: $OUT"
echo
head -80 "$OUT"
