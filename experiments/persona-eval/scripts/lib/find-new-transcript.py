#!/usr/bin/env python3
"""Find the newest *.jsonl transcript that appeared after a snapshot file.

Usage: find-new-transcript.py <transcript_dir> <snapshot_file>

Prints the path of the newest new transcript, or an empty line if none.
Exit code is always 0 — absence of new transcripts is normal, not an error.
"""
import glob
import os
import sys


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: find-new-transcript.py <transcript_dir> <snapshot_file>", file=sys.stderr)
        return 2

    transcript_dir, snapshot_file = sys.argv[1], sys.argv[2]

    known: set[str] = set()
    if os.path.exists(snapshot_file):
        with open(snapshot_file) as f:
            known = {line.strip() for line in f if line.strip()}

    current = set(glob.glob(os.path.join(transcript_dir, "*.jsonl")))
    new_files = current - known

    if new_files:
        newest = max(new_files, key=os.path.getmtime)
        print(newest)
    else:
        print("")
    return 0


if __name__ == "__main__":
    sys.exit(main())
