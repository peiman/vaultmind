#!/bin/bash
# mutation-check.sh — prove a test is load-bearing by breaking the code it guards.
#
# A test that passes is not evidence. A test that passes *because it cannot
# fail* is worse than no test: it occupies the slot where a real check would
# go, and it reports green forever. Session 07 shipped two of them — both
# asserted `> 0` against a fixture where the broken and the fixed
# implementation give the same answer — which is exactly why the two bugs
# underneath them survived a year of green runs.
#
# The ritual is: break the production code the test claims to guard, and watch
# the test go red. If it stays green, the test is decorative. Doing that by
# hand is three steps, and step three is the dangerous one: on 2026-08-19 a
# hand-run `git checkout -- <file>` restored from HEAD instead of from the
# working state and destroyed an hour of uncommitted work. This script
# restores from a byte copy it took itself, so the blast radius is the
# mutation and nothing else.
#
# Usage:
#   mutation-check.sh --file <path> --from <literal> --to <literal> \
#                     --test <regex> [--pkg <pattern>] [--tags <tags>]
#
# --from/--to are LITERAL strings, not regexes. Go source is full of `.`, `(`,
# `[`, backticks and quotes; making the caller escape them is how a mutation
# silently fails to apply.
#
# Exit codes — the distinction between 1 and 2 is the whole point:
#   0  the test FAILED under mutation ....... the test is load-bearing (good)
#   1  the test PASSED under mutation ....... the test cannot fail (a finding)
#   2  the run proves NOTHING ............... mutation never applied, or the
#                                             -run regex matched no test
#   3  precondition or restore failure ...... source may need manual restore
#
# Exit 2 exists because "no output" and "no problem" are the same shape on a
# terminal. A --from string that isn't in the file, or a --test name with a
# typo, both produce a clean green run that means nothing at all. Three times
# in one session an empty result was read as a clean result; this makes that
# reading impossible.

set -euo pipefail

FILE=""
FROM=""
TO=""
TEST=""
PKG=""
TAGS="dev"

usage() {
    sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'
    exit 3
}

while [ $# -gt 0 ]; do
    case "$1" in
        --file) FILE="${2:-}"; shift 2 ;;
        --from) FROM="${2:-}"; shift 2 ;;
        --to)   TO="${2:-}";   shift 2 ;;
        --test) TEST="${2:-}"; shift 2 ;;
        --pkg)  PKG="${2:-}";  shift 2 ;;
        --tags) TAGS="${2:-}"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "[mutation-check] unknown argument: $1" >&2; usage ;;
    esac
done

# Spelled out rather than looped over variable names: ${var,,} is a bash 4
# expansion and macOS ships bash 3.2, so the loop form would print the flag
# name in the wrong case on the platform this is most often run from.
[ -n "$FILE" ] || { echo "[mutation-check] --file is required" >&2; usage; }
[ -n "$FROM" ] || { echo "[mutation-check] --from is required" >&2; usage; }
[ -n "$TEST" ] || { echo "[mutation-check] --test is required" >&2; usage; }

if [ ! -f "$FILE" ]; then
    echo "[mutation-check] no such file: $FILE" >&2
    exit 3
fi

# --to may legitimately be empty (deleting a clause is a valid mutation), but
# it must differ from --from or the "mutation" is a no-op by construction.
if [ "$FROM" = "$TO" ]; then
    echo "[mutation-check] --from and --to are identical; that mutates nothing" >&2
    exit 3
fi

BACKUP="${FILE}.mutation-backup"

# A leftover backup means a previous run died between mutating and restoring,
# so $FILE currently holds mutated source. Overwriting the backup now would
# make the mutation permanent and unrecoverable. Refuse, and say how to fix it.
if [ -e "$BACKUP" ]; then
    cat >&2 <<EOF
[mutation-check] a backup from an earlier run is still here:
    $BACKUP

  That run did not finish restoring, so $FILE may still hold mutated
  source. Restore it before running again:

    mv "$BACKUP" "$FILE"
EOF
    exit 3
fi

# Default the package to the mutated file's own directory. An absolute --file
# must not get a "./" glued in front of it — ".//Users/..." is not a package
# pattern, and go test's error for it does not point at this line.
if [ -z "$PKG" ]; then
    PKG="$(dirname "$FILE")"
    case "$PKG" in
        /*) ;;
        *) PKG="./$PKG" ;;
    esac
fi

sum_of() {
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 < "$1" | cut -d' ' -f1
    else
        sha256sum < "$1" | cut -d' ' -f1
    fi
}

# Taken BEFORE the backup is made, and checked again after restore. The backup
# file is consumed by the restore itself, so it cannot also serve as the thing
# the restored file is compared against.
ORIG_SUM="$(sum_of "$FILE")"

cp -p "$FILE" "$BACKUP"
COUNT_FILE="$(mktemp)"

restore() {
    rm -f "$COUNT_FILE"
    if [ -e "$BACKUP" ]; then
        mv "$BACKUP" "$FILE"
    fi
}
trap restore EXIT

# Literal replace. \Q..\E quotes every metacharacter in the pattern, and the
# replacement is interpolated once from a variable, so neither side needs the
# caller to escape anything. The substitution count goes to stderr so an
# unexpectedly broad --from is visible rather than silent — reading source from
# $BACKUP and writing to $FILE means the redirection truncating $FILE cannot
# race the read.
MC_FROM="$FROM" MC_TO="$TO" perl -0777 -e '
    my $src = do { local $/; <STDIN> };
    my $n = ($src =~ s/\Q$ENV{MC_FROM}\E/$ENV{MC_TO}/g);
    print STDERR $n + 0;
    print STDOUT $src;
' < "$BACKUP" > "$FILE" 2>"$COUNT_FILE"
SITES="$(cat "$COUNT_FILE")"

# THE guard: if the file is byte-identical to what it was, nothing was
# mutated, and whatever the test does next says nothing about the test.
if cmp -s "$BACKUP" "$FILE"; then
    cat >&2 <<EOF
[mutation-check] NOTHING WAS MUTATED — this run proves nothing.

  file: $FILE
  from: $FROM

  The --from string does not appear in the file. A test run now would pass
  against unmodified source, which looks exactly like a test that survived
  a mutation. It is not the same thing.
EOF
    exit 2
fi

echo "[mutation-check] mutated $SITES site(s) in $FILE"
echo "[mutation-check] running: go test -count=1 -tags \"$TAGS\" -run '$TEST' $PKG"
echo

# -count=1 defeats the test cache. A cached PASS under a mutated tree is the
# purest form of the lie this script exists to catch.
set +e
OUTPUT=$(go test -count=1 -tags "$TAGS" -run "$TEST" -v "$PKG" 2>&1)
TEST_STATUS=$?
set -e

echo "$OUTPUT"
echo

# A mutation that does not compile is the commonest way to get here, and it is
# a different problem from a mistyped test name. Both leave zero RUN lines, so
# naming the wrong one sends you looking in the wrong place.
if grep -qE '\[build failed\]|^# ' <<<"$OUTPUT"; then
    cat >&2 <<EOF
[mutation-check] THE MUTATION DID NOT COMPILE — this run proves nothing.

  from: $FROM
  to:   $TO

  Nothing was tested, so nothing was learned about the test. Pick a mutation
  that is still valid Go — invert a comparison, change a constant, drop a
  clause — rather than one that breaks the parse.
EOF
    exit 2
fi

# The second half of the "did it run?" guard. `go test -run TestTpyo` exits 0
# and prints "no tests to run" — indistinguishable from a green suite unless
# you look for the RUN lines.
if ! grep -q '^=== RUN' <<<"$OUTPUT"; then
    cat >&2 <<EOF
[mutation-check] NO TEST RAN — this run proves nothing.

  -run '$TEST' matched no test in $PKG. go test exits 0 for that, which
  reads as a pass. Check the test name and the package.
EOF
    exit 2
fi

restore
trap - EXIT

if [ "$(sum_of "$FILE")" != "$ORIG_SUM" ]; then
    cat >&2 <<EOF
[mutation-check] RESTORE FAILED — $FILE does not match what it was.

  Do not trust the working tree. If a backup survives it is at:
    $BACKUP
  Otherwise recover with: git diff -- "$FILE"
EOF
    exit 3
fi

if [ "$TEST_STATUS" -ne 0 ]; then
    echo "[mutation-check] PASS — the test failed under mutation, so it is load-bearing."
    exit 0
fi

cat >&2 <<EOF
[mutation-check] FINDING — the test PASSED with the code broken.

  file: $FILE
  from: $FROM
  to:   $TO
  test: $TEST

  The mutation applied ($SITES site(s)) and the test still went green. It is
  not guarding this behaviour. Either the assertion is true under both the
  broken and the fixed implementation (the \`> 0\` shape), or the fixture never
  reaches the mutated line.
EOF
exit 1
