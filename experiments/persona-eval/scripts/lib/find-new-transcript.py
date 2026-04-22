#!/usr/bin/env python3
"""Find the oldest *.jsonl transcript that appeared after a snapshot file,
has at least N user messages, and is not in the --exclude list.

Usage:
  find-new-transcript.py <transcript_dir> <snapshot_file> [options]

Options:
  --exclude <path>       Path to exclude from consideration (repeatable).
                         Use for the current session's own transcript_path so
                         it is never mistaken for a completed prior session.
  --min-user-msgs <N>    Require at least N lines with type=="user" (default 1).
                         Filters out abandoned windows that have only
                         permission-mode / attachment events.

Prints the path of the OLDEST eligible new transcript (by mtime), or empty.
Oldest means "first completed session after the slot started" — the work
session, not a later advance/throwaway window. Exit is always 0.
"""
from __future__ import annotations

import argparse
import glob
import json
import os
import sys


def count_user_messages(path: str) -> int:
    n = 0
    try:
        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    d = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if d.get("type") == "user":
                    n += 1
    except OSError:
        return 0
    return n


def main() -> int:
    p = argparse.ArgumentParser(add_help=False)
    p.add_argument("transcript_dir")
    p.add_argument("snapshot_file")
    p.add_argument("--exclude", action="append", default=[])
    p.add_argument("--min-user-msgs", type=int, default=1)
    args = p.parse_args()

    known: set[str] = set()
    if os.path.exists(args.snapshot_file):
        with open(args.snapshot_file) as f:
            known = {line.strip() for line in f if line.strip()}

    excluded = {os.path.realpath(x) for x in args.exclude if x}
    current = {os.path.realpath(x) for x in glob.glob(os.path.join(args.transcript_dir, "*.jsonl"))}
    known_real = {os.path.realpath(x) for x in known}
    candidates = current - known_real - excluded

    eligible = [c for c in candidates if count_user_messages(c) >= args.min_user_msgs]

    if eligible:
        oldest = min(eligible, key=os.path.getmtime)
        print(oldest)
    else:
        print("")
    return 0


if __name__ == "__main__":
    sys.exit(main())
