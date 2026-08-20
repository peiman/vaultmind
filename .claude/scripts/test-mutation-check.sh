#!/bin/bash
# Tests for mutation-check.sh.
#
# The script exists to answer "can this test fail?", so the first thing to
# establish about it is that IT can fail — a checker that reports success on
# every input is the exact defect it was built to catch.
#
# It runs against a throwaway Go module built here, not against the repo, so
# the expected outcomes are known rather than assumed: a load-bearing test that
# must go red, a decorative `> 0` test that must stay green, and the two ways a
# run can prove nothing at all.
#
# Run directly: bash .claude/scripts/test-mutation-check.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/mutation-check.sh"

if [ ! -f "$SCRIPT" ]; then
    echo "FAIL: $SCRIPT does not exist" >&2
    exit 1
fi

if ! command -v go >/dev/null 2>&1; then
    echo "FAIL: go is not on PATH — these tests need the Go toolchain" >&2
    exit 1
fi

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

cat > "$FIXTURE/go.mod" <<'EOF'
module mutfixture

go 1.22
EOF

cat > "$FIXTURE/calc.go" <<'EOF'
package mutfixture

func Double(n int) int {
	return n * 2
}
EOF

# TestDoubleExact pins the answer, so n*3 breaks it. TestDoublePositive is the
# `> 0` shape that survived twice in session 07 — true whether Double doubles
# or triples, which is what makes it decorative.
cat > "$FIXTURE/calc_test.go" <<'EOF'
package mutfixture

import "testing"

func TestDoubleExact(t *testing.T) {
	if got := Double(3); got != 6 {
		t.Fatalf("Double(3) = %d, want 6", got)
	}
}

func TestDoublePositive(t *testing.T) {
	if got := Double(3); got <= 0 {
		t.Fatalf("Double(3) = %d, want positive", got)
	}
}
EOF

SOURCE="$FIXTURE/calc.go"
PRISTINE="$(shasum -a 256 < "$SOURCE" | cut -d' ' -f1)"

FAILURES=0

# Every case asserts the exit code AND that the source came back byte-identical.
# Restoring is the half that destroyed real work when done by hand, so it is
# not enough for the verdict to be right.
LAST_OUTPUT=""

expect() {
    local name="$1" want="$2"; shift 2
    local got=0
    set +e
    LAST_OUTPUT=$( cd "$FIXTURE" && bash "$SCRIPT" "$@" 2>&1 )
    got=$?
    set -e

    if [ "$got" -ne "$want" ]; then
        echo "FAIL: $name — exit $got, want $want" >&2
        FAILURES=$((FAILURES + 1))
    elif [ "$(shasum -a 256 < "$SOURCE" | cut -d' ' -f1)" != "$PRISTINE" ]; then
        echo "FAIL: $name — exit code was right but the source was not restored" >&2
        FAILURES=$((FAILURES + 1))
    elif [ -e "$SOURCE.mutation-backup" ]; then
        echo "FAIL: $name — left a backup file behind" >&2
        FAILURES=$((FAILURES + 1))
    else
        echo "ok: $name"
    fi
}

# Asserts on the output of the expect() immediately above it, for the cases
# where two different guards produce the same exit code and only the wording
# tells the caller which one fired.
expect_says() {
    local name="$1" want="$2"
    if grep -q "$want" <<<"$LAST_OUTPUT"; then
        echo "ok: $name"
    else
        echo "FAIL: $name — output did not contain '$want'" >&2
        FAILURES=$((FAILURES + 1))
    fi
}

expect "a load-bearing test goes red under mutation" 0 \
    --file calc.go --from "return n * 2" --to "return n * 3" \
    --test "TestDoubleExact" --pkg . --tags ""

expect "a '> 0' test stays green and is reported as a finding" 1 \
    --file calc.go --from "return n * 2" --to "return n * 3" \
    --test "TestDoublePositive" --pkg . --tags ""

expect "a --from that is not in the file proves nothing" 2 \
    --file calc.go --from "return n * 99" --to "return n * 3" \
    --test "TestDoubleExact" --pkg . --tags ""

expect "a --test that matches no test proves nothing" 2 \
    --file calc.go --from "return n * 2" --to "return n * 3" \
    --test "TestDoubleExactTpyo" --pkg . --tags ""

# Exit 2 alone cannot vouch for this one: a mutation that fails to compile
# produces no RUN lines either, so the "no test ran" guard would return 2 even
# with the compile guard deleted. Asserting only the code would be the `> 0`
# shape — true under both implementations. The message is what differs, so the
# message is what gets asserted.
expect "a mutation that does not compile proves nothing" 2 \
    --file calc.go --from "return n * 2" --to "return n *" \
    --test "TestDoubleExact" --pkg . --tags ""
expect_says "and says so, rather than blaming the test name" "DID NOT COMPILE"

expect "an absolute --file resolves its own package" 0 \
    --file "$SOURCE" --from "return n * 2" --to "return n * 3" \
    --test "TestDoubleExact" --tags ""

expect "--from equal to --to is refused" 3 \
    --file calc.go --from "return n * 2" --to "return n * 2" \
    --test "TestDoubleExact" --pkg . --tags ""

expect "a missing file is refused" 3 \
    --file nosuch.go --from "a" --to "b" --test "TestDoubleExact" --pkg . --tags ""

# A leftover backup means an earlier run died mid-mutation. Overwriting it
# would make the mutation permanent, so the script must refuse and leave both
# files exactly as it found them.
echo "stale backup contents" > "$SOURCE.mutation-backup"
set +e
( cd "$FIXTURE" && bash "$SCRIPT" --file calc.go --from "return n * 2" --to "return n * 3" \
    --test "TestDoubleExact" --pkg . --tags "" ) >/dev/null 2>&1
LEFTOVER_STATUS=$?
set -e
if [ "$LEFTOVER_STATUS" -ne 3 ]; then
    echo "FAIL: a leftover backup must be refused — exit $LEFTOVER_STATUS, want 3" >&2
    FAILURES=$((FAILURES + 1))
elif [ "$(cat "$SOURCE.mutation-backup")" != "stale backup contents" ]; then
    echo "FAIL: a leftover backup must not be overwritten" >&2
    FAILURES=$((FAILURES + 1))
else
    echo "ok: a leftover backup is refused, and left intact"
fi
rm -f "$SOURCE.mutation-backup"

if [ "$FAILURES" -ne 0 ]; then
    echo >&2
    echo "$FAILURES mutation-check test(s) failed" >&2
    exit 1
fi

echo
echo "all mutation-check tests passed"
