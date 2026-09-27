#!/bin/bash
# test-staged.sh — run the tests of the packages a commit touches.
#
# Pre-commit used to run the whole suite, and pre-push ran it again under
# coverage: a commit plus a push took up to ten minutes (#185). Pre-commit now
# tests only the packages owning the staged files; pre-push keeps the full,
# authoritative coverage run, so nothing reaches the remote untested.
#
# A file maps to the nearest directory at or above it that holds Go code —
# so an embedded hook script or template tests the package that embeds it. A
# non-Go file at the repository root belongs to no package. go.mod or go.sum
# can change anything, so they select everything.
#
# Usage: test-staged.sh [--list] <file>...
#   --list  print the selected packages and run nothing (used by the tests)

set -euo pipefail

list_only=0
if [ "${1:-}" = "--list" ]; then
    list_only=1
    shift
fi

has_go() { compgen -G "$1/*.go" >/dev/null 2>&1; }

packages=""
for f in "$@"; do
    case "$f" in
        go.mod | go.sum)
            packages="./..."
            break
            ;;
    esac
    dir=$(dirname "$f")
    while [ "$dir" != "." ] && [ "$dir" != "/" ] && ! has_go "$dir"; do
        dir=$(dirname "$dir")
    done
    if [ "$dir" = "." ]; then
        # The root: only a Go file there selects the root package.
        case "$f" in */*) continue ;; *.go) packages="$packages ." ;; esac
        continue
    fi
    packages="$packages ./$dir"
done

if [ "$packages" != "./..." ]; then
    packages=$(printf '%s\n' $packages | sort -u | tr '\n' ' ' | sed 's/ *$//')
fi

if [ "$list_only" = 1 ]; then
    [ -n "$packages" ] && printf '%s\n' $packages
    exit 0
fi

if [ -z "$packages" ]; then
    echo "test-staged: no Go package touched — nothing to test (pre-push runs the full suite)"
    exit 0
fi

# Integration tests are not unit tests: test:unit leaves them out, and so does
# this, whether selected by name or by "everything".
if [ "$packages" = "./..." ]; then
    packages=$(go list ./... | grep -v /test/integration | tr '\n' ' ')
fi
packages=$(printf '%s\n' $packages | grep -v '/test/integration' | tr '\n' ' ' | sed 's/ *$//')
if [ -z "$packages" ]; then
    echo "test-staged: only integration tests touched — pre-push and CI run them"
    exit 0
fi

export XDG_DATA_HOME="${XDG_DATA_HOME:-$(mktemp -d -t vm-test-staged.XXXXXX)}"
trap '[ -n "$XDG_DATA_HOME" ] && [ -d "$XDG_DATA_HOME" ] && rm -rf "$XDG_DATA_HOME"' EXIT
echo "test-staged: $packages"
# shellcheck disable=SC2086 # word-splitting the package list is the point
exec gotestsum --format short --hide-summary=skipped -- -short $packages
