#!/usr/bin/env python3
"""Fix broken Obsidian wikilinks reported by `vaultmind doctor`.

Many vault concept files are stored as `concepts/<slug>.md` while their `id:`
frontmatter still carries the `concept-<slug>` form. Cross-note wikilinks
written as `[[concept-<slug>]]` therefore fail to resolve in Obsidian — there
is no `concept-<slug>.md` file. The fix is to rewrite the link's *target*
portion (everything before the optional `|display` part) from `concept-<slug>`
to `<slug>`, preserving any existing display text.

We trust `vaultmind doctor` to identify the (broken-target, correct-target)
pairs in its `→` output. We then regex-rewrite every wikilink `[[<old>]]` /
`[[<old>|...]]` to `[[<new>]]` / `[[<new>|...]]` in the same file.

Idempotent: running twice on a clean vault is a no-op.

Usage:
    /tmp/vaultmind doctor --vault vaultmind-vault 2>&1 \
        | python3 scripts/fix_wikilinks.py --vault vaultmind-vault
"""
import argparse
import re
import sys
from pathlib import Path

# Captures: 1=relative path, 2=old target, 3=new target.
ARROW_LINE = re.compile(
    r"^\s*(\S+\.md):\s+\[\[([^\]|]+)(?:\|[^\]]*)?\]\]\s+→\s+\[\[([^\]|]+)(?:\|[^\]]*)?\]\]"
)


def rewrite_link(text, old_target, new_target):
    """Rewrite [[old_target]] and [[old_target|display]] → new_target form,
    preserving any display text. Returns (new_text, count)."""
    # Match the literal old target only when it's the *target* of a wikilink
    # (immediately after `[[` and followed by either `]]` or `|`).
    pattern = re.compile(
        r"\[\[" + re.escape(old_target) + r"(\]\]|\|)"
    )
    new_text, count = pattern.subn(r"[[" + new_target + r"\1", text)
    return new_text, count


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("--vault", default="vaultmind-vault", help="Vault root")
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would change without writing",
    )
    args = parser.parse_args()
    vault = Path(args.vault)
    if not vault.is_dir():
        print(f"error: {vault} is not a directory", file=sys.stderr)
        return 2

    edits = {}  # rel_path -> set of (old_target, new_target)
    for line in sys.stdin:
        m = ARROW_LINE.match(line)
        if not m:
            continue
        rel_path, old_t, new_t = m.group(1), m.group(2), m.group(3)
        edits.setdefault(rel_path, set()).add((old_t, new_t))

    if not edits:
        print("no rewrites found on stdin (already clean?)")
        return 0

    total_files = 0
    total_links = 0
    for rel_path, pairs in sorted(edits.items()):
        full = vault / rel_path
        if not full.is_file():
            print(f"skip: {rel_path} does not exist under {vault}", file=sys.stderr)
            continue
        text = full.read_text()
        original = text
        applied = 0
        for old_t, new_t in pairs:
            text, n = rewrite_link(text, old_t, new_t)
            applied += n
        if text == original:
            continue
        total_files += 1
        total_links += applied
        if args.dry_run:
            print(f"[dry-run] {rel_path}: {applied} link(s)")
        else:
            full.write_text(text)
            print(f"fixed:   {rel_path}: {applied} link(s)")

    print(f"\n{total_links} link(s) across {total_files} file(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
