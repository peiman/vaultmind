#!/bin/bash
# Tests for test-staged.sh — the pre-commit test selection.
#
# The selection decides which tests a commit runs, so a wrong mapping skips
# the tests that would have caught the change. These cases run against the
# repository's own layout (via --list, which runs nothing), so each expected
# package is a real one.
#
# Run directly: bash .claude/scripts/test-test-staged.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/test-staged.sh"
cd "$SCRIPT_DIR/../.."

if [ ! -f "$SCRIPT" ]; then
    echo "FAIL: $SCRIPT does not exist" >&2
    exit 1
fi

failures=0
expect() {
    local want="$1"
    shift
    local got
    got=$(bash "$SCRIPT" --list "$@" | tr '\n' ' ' | sed 's/ *$//')
    if [ "$got" = "$want" ]; then
        echo "ok: $* -> ${want:-<none>}"
    else
        echo "FAIL: $* -> got '${got:-<none>}', want '${want:-<none>}'" >&2
        failures=$((failures + 1))
    fi
}

# A Go file tests its own package.
expect "./internal/navigate" internal/navigate/navigate.go
# An embedded hook script tests the package that embeds it.
expect "./internal/hookscripts" internal/hookscripts/vault-reach.sh
# A template deep under a package walks up to that package.
expect "./internal/initvault" internal/initvault/knowledge/README.md
# Two files in one package test it once.
expect "./internal/navigate" internal/navigate/navigate.go internal/navigate/covering.go
# Files in different packages test each, sorted.
expect "./internal/navigate ./internal/query" internal/query/ask.go internal/navigate/navigate.go
# The root package is tested for a Go file at the root.
expect "." main.go
# A non-Go file at the root belongs to no package: nothing to run.
expect "" CHANGELOG.md
expect "" .claude/scripts/test-staged.sh
# A deleted file still names its package.
expect "./internal/navigate" internal/navigate/no-longer-here.go
# A dependency change can affect anything: everything runs.
expect "./..." go.mod
expect "./..." internal/navigate/navigate.go go.sum

if [ "$failures" -gt 0 ]; then
    echo "$failures case(s) failed" >&2
    exit 1
fi
echo "all test-staged cases passed"
