#!/usr/bin/env python3
"""Find the newest *.jsonl transcript that appeared after a snapshot file.

Usage: find-new-transcript.py <transcript_dir> <snapshot_file> [min_age_seconds]

Prints the path of the newest new transcript whose mtime is at least
min_age_seconds old (default 30), or an empty line if none. The age filter
protects against mis-identifying the currently-active session's transcript
(which is being written right now) as a completed prior session's output.
Exit code is always 0 — absence of new transcripts is normal, not an error.
"""
import glob
import os
import sys
import time


def main() -> int:
    if len(sys.argv) not in (3, 4):
        print("usage: find-new-transcript.py <transcript_dir> <snapshot_file> [min_age_seconds]", file=sys.stderr)
        return 2

    transcript_dir, snapshot_file = sys.argv[1], sys.argv[2]
    min_age_seconds = float(sys.argv[3]) if len(sys.argv) == 4 else 30.0

    known: set[str] = set()
    if os.path.exists(snapshot_file):
        with open(snapshot_file) as f:
            known = {line.strip() for line in f if line.strip()}

    current = set(glob.glob(os.path.join(transcript_dir, "*.jsonl")))
    new_files = current - known

    now = time.time()
    eligible = [f for f in new_files if (now - os.path.getmtime(f)) >= min_age_seconds]

    if eligible:
        newest = max(eligible, key=os.path.getmtime)
        print(newest)
    else:
        print("")
    return 0


if __name__ == "__main__":
    sys.exit(main())
