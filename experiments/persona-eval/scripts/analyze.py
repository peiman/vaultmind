#!/usr/bin/env python3
"""Analyze persona eval experiment results.

Reads score files from results/, cross-references with schedule.json,
produces a markdown report with per-condition stats, effect sizes,
and inter-rater agreement.

Usage:
    python3 analyze.py
    python3 analyze.py --all-runs
"""

import argparse
import glob
import json
import sys
from collections import defaultdict
from pathlib import Path

import numpy as np
import pandas as pd
from scipy import stats as scipy_stats

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
    u_stat, _ = scipy_stats.mannwhitneyu(group1, group2, alternative="two-sided")
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
            u, p = scipy_stats.mannwhitneyu(g1, g2, alternative="two-sided")
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
        u, p = scipy_stats.mannwhitneyu(g_a, g_c, alternative="two-sided")
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
