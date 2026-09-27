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
# Shared fixtures are read by many packages' tests: everything runs.
expect "./..." test/fixtures/testvault/concepts/x.md
expect "./..." testdata/config/valid.yaml
# An installed hook copy is compared with the embedded original.
expect "./internal/hookscripts" .claude/scripts/vault-reach.sh
# A folder of build-constrained Go files is not a package: nothing to run.
expect "" .ckeletin/scripts/lint.sh

# Without --list: an integration-only commit runs nothing, and says why.
got=$(bash "$SCRIPT" test/integration/integration_test.go 2>&1) && rc=0 || rc=$?
if [ "$rc" = 0 ] && echo "$got" | grep -q "only integration tests touched"; then
    echo "ok: integration-only commit -> nothing, exit 0"
else
    echo "FAIL: integration-only commit -> exit $rc: $got" >&2
    failures=$((failures + 1))
fi

# With no files named, the staged files come from git — including a deleted
# file and the old side of a rename, which lefthook does not pass.
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
(
    unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_PREFIX GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
    cd "$scratch"
    git init -q .
    printf 'module scratch\n\ngo 1.21\n' >go.mod
    mkdir a b c
    printf 'package a\n' >a/one.go
    printf 'package a\n' >a/two.go
    printf 'package b\n' >b/keep.go
    # c keeps a file after the move, so it is still a package to test.
    printf 'package b\n' >c/moved.go
    printf 'package b\n' >c/stays.go
    git add . && git -c user.email=t@t -c user.name=t commit -qm init
    git rm -q a/one.go
    git mv c/moved.go b/moved.go
    bash "$SCRIPT" --list | tr '\n' ' ' | sed 's/ *$//' >"$scratch/got"
)
got=$(cat "$scratch/got")
if [ "$got" = "./a ./b ./c" ]; then
    echo "ok: staged from git -> ./a ./b ./c"
else
    echo "FAIL: staged from git -> got '$got', want './a ./b ./c'" >&2
    failures=$((failures + 1))
fi

if [ "$failures" -gt 0 ]; then
    echo "$failures case(s) failed" >&2
    exit 1
fi
echo "all test-staged cases passed"
