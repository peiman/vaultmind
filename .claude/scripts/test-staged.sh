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
# directory whose Go files are all build-constrained out (.ckeletin/scripts) is
# not a package, and selects nothing. Then the exceptions:
#   - go.mod or go.sum can change anything: everything runs.
#   - Shared fixtures (test/fixtures, testdata, examples) are read by tests in
#     many packages through relative paths: everything runs.
#   - An installed hook copy in .claude/scripts is compared with its embedded
#     original: internal/hookscripts runs.
#   - Any other non-Go file outside a package selects nothing.
#
# With no files named, the staged files are read from git — deletions and
# both sides of a rename included, which lefthook's {staged_files} leaves out.
#
# Usage: test-staged.sh [--list] [<file>...]
#   --list  print the selected packages and run nothing (used by the tests)

set -euo pipefail

list_only=0
if [ "${1:-}" = "--list" ]; then
    list_only=1
    shift
fi

if [ "$#" -eq 0 ]; then
    staged=$(git diff --cached --name-only --no-renames --diff-filter=ACMRD)
    # shellcheck disable=SC2086 # one path per word; the repository has no paths with spaces
    set -- $staged
fi

has_go() { compgen -G "$1/*.go" >/dev/null 2>&1; }

everything=0
packages=""
for f in "$@"; do
    case "$f" in
        go.mod | go.sum | test/fixtures/* | testdata/* | examples/*)
            everything=1
            break
            ;;
        .claude/scripts/*)
            if [ -f "internal/hookscripts/$(basename "$f")" ]; then
                packages="$packages ./internal/hookscripts"
            fi
            continue
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

if [ "$everything" = 1 ]; then
    packages="./..."
elif [ -n "$packages" ]; then
    # Keep only real packages: a directory of build-constrained Go files is
    # not one, and `go test` on it fails.
    # shellcheck disable=SC2046,SC2086 # word-splitting the package list is the point
    packages=$(go list -e -f '{{if not .Error}}{{.Dir}}{{end}}' $(printf '%s\n' $packages | sort -u) |
        sed "s|^$(pwd)|.|" | sort -u | tr '\n' ' ' | sed 's/ *$//')
fi

if [ "$list_only" = 1 ]; then
    # shellcheck disable=SC2086 # one package per line
    if [ -n "$packages" ]; then printf '%s\n' $packages; fi
    exit 0
fi

if [ -z "$packages" ]; then
    echo "test-staged: no Go package touched — nothing to test (pre-push runs the full suite)"
    exit 0
fi

# Integration tests are not unit tests: test:unit leaves them out, and so does
# this, whether selected by name or by "everything".
if [ "$packages" = "./..." ]; then
    packages=$(go list ./... | tr '\n' ' ')
fi
# shellcheck disable=SC2086 # one package per line
packages=$(printf '%s\n' $packages | { grep -v '/test/integration' || true; } | tr '\n' ' ' | sed 's/ *$//')
if [ -z "$packages" ]; then
    echo "test-staged: only integration tests touched — pre-push and CI run them"
    exit 0
fi

# An isolated data dir, removed afterwards — unless the caller supplied one,
# which is theirs to keep.
if [ -z "${XDG_DATA_HOME:-}" ]; then
    XDG_DATA_HOME=$(mktemp -d -t vm-test-staged.XXXXXX)
    export XDG_DATA_HOME
    trap 'rm -rf "$XDG_DATA_HOME"' EXIT
fi
echo "test-staged: $packages"
# shellcheck disable=SC2086 # word-splitting the package list is the point
gotestsum --format short --hide-summary=skipped -- -short $packages
