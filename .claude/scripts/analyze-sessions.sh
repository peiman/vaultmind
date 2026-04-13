#!/bin/bash
# Analyze persona evaluation sessions.
# Joins injection logs with human scores by session ID.
# Computes per-injection success rate.

python3 << 'PYEOF'
import json, glob, os

LOG_DIR = os.path.expanduser("~/.vaultmind/persona-eval")
SCORES_FILE = os.path.join(LOG_DIR, "human-scores.tsv")

# Load injection logs
injections = {}
for f in sorted(glob.glob(os.path.join(LOG_DIR, "*-injection.json"))):
    try:
        with open(f) as fh:
            d = json.load(fh)
        sid = d.get("session_id", "unknown")
        d["_file"] = os.path.basename(f)
        injections[sid] = d
    except:
        pass

# Load human scores
scores = {}
if os.path.exists(SCORES_FILE):
    with open(SCORES_FILE) as fh:
        header = None
        for line in fh:
            parts = line.strip().split("\t")
            if header is None:
                header = parts
                continue
            if "session_id" in header and len(parts) >= 3:
                # New format: timestamp, session_id, grade, note
                sid = parts[1]
                scores[sid] = {"timestamp": parts[0], "grade": parts[2], "note": parts[3] if len(parts) > 3 else ""}
            elif len(parts) >= 2:
                # Old format: timestamp, grade, note (no session ID)
                scores[f"old_{parts[0]}"] = {"timestamp": parts[0], "grade": parts[1], "note": parts[2] if len(parts) > 2 else "", "no_session_id": True}

# Print report
print("=" * 60)
print("  PERSONA EVALUATION — SESSION ANALYSIS")
print("=" * 60)
print()

total_inj = len(injections)
success_inj = sum(1 for i in injections.values() if i.get("injection_success"))
failed_inj = total_inj - success_inj
total_scores = len(scores)

print(f"  Injection logs:  {total_inj} ({success_inj} success, {failed_inj} failed)")
print(f"  Human scores:    {total_scores}")
print()

# Match by session ID
paired = []
for sid, inj in sorted(injections.items(), key=lambda x: x[1].get("timestamp", "")):
    sc = scores.get(sid)
    paired.append({"session_id": sid, "injection": inj, "score": sc})

unmatched_scores = {k: v for k, v in scores.items() if k not in injections and not v.get("no_session_id")}
old_scores = {k: v for k, v in scores.items() if v.get("no_session_id")}

# Paired sessions table
if paired:
    print("=" * 60)
    print("  PAIRED SESSIONS")
    print("=" * 60)
    print()
    print(f"  {'Session':<12} {'Date':<18} {'Injected':<10} {'Grade':<7} {'Identity':<10} {'Note'}")
    print(f"  {'-------':<12} {'----':<18} {'--------':<10} {'-----':<7} {'--------':<10} {'----'}")
    for p in paired:
        inj = p["injection"]
        sid = p["session_id"][:8] + "..."
        ts = inj.get("timestamp", "?")
        injected = "OK" if inj.get("injection_success") else "FAILED"
        id_len = inj.get("identity_length", 0)
        if p["score"]:
            grade = p["score"]["grade"]
            note = p["score"]["note"][:35]
        else:
            grade = "—"
            note = "(no human score yet)"
        print(f"  {sid:<12} {ts:<18} {injected:<10} {grade:<7} {id_len:>5} ch  {note}")
    print()

# Old scores (before session ID tracking)
if old_scores:
    print("=" * 60)
    print("  OLD SCORES (before session ID tracking)")
    print("=" * 60)
    print()
    for k, sc in old_scores.items():
        print(f"  {sc['timestamp']:<18} {sc['grade']:<7} {sc.get('note', '')[:50]}")
    print()

# Per-injection success rate
print("=" * 60)
print("  PER-INJECTION SUCCESS RATE")
print("=" * 60)
print()

scored_and_injected = [p for p in paired if p["score"] and p["injection"].get("injection_success")]
if scored_and_injected:
    total = len(scored_and_injected)
    a_count = sum(1 for p in scored_and_injected if p["score"]["grade"] == "A")
    b_count = sum(1 for p in scored_and_injected if p["score"]["grade"] == "B")
    c_count = sum(1 for p in scored_and_injected if p["score"]["grade"] == "C")
    success = b_count + c_count
    rate = success / total * 100

    print(f"  Verified injection + human score: {total} sessions")
    print()
    print(f"    A (stranger):  {a_count}")
    print(f"    B (knows me):  {b_count}")
    print(f"    C (partner):   {c_count}")
    print()
    print(f"  Per-injection success rate (B+C): {success}/{total} = {rate:.0f}%")
    print()

    if total < 20:
        remaining = 20 - total
        print(f"  ⚠  {remaining} more sessions needed for minimum sample.")
    else:
        print(f"  ✓  Sample size reached ({total} sessions).")
        print()
        if rate >= 80:
            print("  → Injection works. Proceed to content optimization.")
        elif rate >= 50:
            print("  → Stochastic. Investigate success vs failure patterns.")
        else:
            print("  → Unreliable. Rethink content format first.")
else:
    print("  No paired data yet.")
    print("  Start sessions, check hook, score turn-1. They link by session ID now.")

print()
print("=" * 60)
print("  DECISION GATES")
print("=" * 60)
print()
print("  >80% B+C → Injection works, optimize content")
print("  50-80%   → Stochastic, investigate patterns")
print("  <50%     → Unreliable, rethink format")
print()
PYEOF
