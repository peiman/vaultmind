#!/bin/bash
# Bash 3.2 compatible (macOS ships 3.2). Uses parallel arrays instead of
# associative arrays for per-package accounting.
# Check if project coverage meets minimum threshold
# Similar to codecov/project check
#
# Excludes from coverage calculation:
# - _tui.go files (TUI code requires interactive testing)
# - /demo/ directories (demo code is for documentation)

set -eo pipefail

COVERAGE_FILE="${COVERAGE_FILE:-coverage.txt}"
MIN_COVERAGE="${MIN_COVERAGE:-85.0}"

if [ ! -f "$COVERAGE_FILE" ]; then
    echo "❌ Coverage file not found: $COVERAGE_FILE"
    echo "Run 'task test' first to generate coverage data"
    exit 1
fi

# Calculate coverage ourselves, excluding TUI and demo code
# Format: file:line.col,line.col numStatements numHits
#
# When using -coverpkg=./..., each test binary emits coverage data for ALL
# packages. gocovmerge can produce duplicate entries for the same block with
# different hit counts (e.g., 0 from binaries that don't touch the code, >0
# from the package's own tests). We deduplicate by taking the maximum hit
# count per unique block before calculating coverage.
#
# Exclusions:
#   - *_tui.go: Legacy TUI file naming convention
#   - internal/check/executor.go: TUI executor (split from check_tui.go)
#   - internal/check/summary.go: TUI summary rendering (split from check_tui.go)
#   - /demo/: Demo code for documentation
#   - internal/embedding/: ONNX/hugot model inference — exercised in integration
#     tests with real model files, not unit-testable in CI without the runtime.
#     Tracked for future improvement (see AGENTS.md / roadmap).
#   - cmd/dev_progress.go: //go:build dev demo command for progress UI rendering.
#     Timing-sensitive spinners/bars; exercised by eye, not unit tests.
#   - cmd/check.go: //go:build dev wrapper that runs the full test suite via
#     internal/check.Executor. Unit-testing this would mean tests-within-tests.
#
# Per-package ratchets (tier floors below the project floor):
#   Certain packages carry invariants the whole system depends on. A regression
#   that silently dropped their coverage would be invisible in the aggregate
#   but catastrophic in practice. We enforce a higher floor per-package for
#   these "data-integrity spine" and "enforcement-layer" packages. See
#   PACKAGE_FLOORS below. Ratchet discipline: floors may only be raised,
#   never lowered, and a new package added to the tier must first hit its
#   floor before the entry lands.

# Calculate per-package coverage from go test output.
#
# The primary coverage file (coverage.txt) uses -coverpkg=./... which instruments
# each source file once per test binary. This creates overlapping coverage blocks
# with different boundaries that inflate the total statement count and undercount
# actual coverage. For accurate threshold enforcement, we generate a separate
# per-package coverage profile where each package is only instrumented once.
PERPKG_COV=$(mktemp)
trap 'rm -f "$PERPKG_COV"' EXIT

go test -tags dev -coverprofile="$PERPKG_COV" -covermode=atomic ./... ./.ckeletin/pkg/... 2>/dev/null

if [ ! -s "$PERPKG_COV" ]; then
    echo "❌ Failed to generate per-package coverage profile"
    echo "Falling back to primary coverage file"
    PERPKG_COV="$COVERAGE_FILE"
fi

# Per-package floors (ratchets at reality; raise over time, never lower).
# Parallel arrays for Bash 3.2 compatibility.
PKG_FLOOR_KEYS=(
    "github.com/peiman/vaultmind/internal/envelope"
    "github.com/peiman/vaultmind/internal/parser"
    "github.com/peiman/vaultmind/internal/schema"
    "github.com/peiman/vaultmind/internal/config/commands"
    "github.com/peiman/vaultmind/internal/vault"
)
PKG_FLOOR_VALUES=(
    100
    100
    100
    100
    90
)

# Running per-package accumulators as parallel arrays. We collect as we walk
# the coverage profile, then look each up when evaluating floors.
PKG_ACC_KEYS=()
PKG_ACC_TOTAL=()
PKG_ACC_COVERED=()

pkg_index() {
    local target="$1"
    local i=0
    for k in "${PKG_ACC_KEYS[@]}"; do
        if [ "$k" = "$target" ]; then
            echo "$i"
            return 0
        fi
        i=$((i + 1))
    done
    echo "-1"
}

total_statements=0
covered_statements=0

while IFS= read -r line; do
    # Skip mode line, TUI, and demo code
    if [[ "$line" == mode:* ]] || [[ "$line" == *"_tui.go"* ]] || [[ "$line" == *"/demo/"* ]] \
       || [[ "$line" == *"internal/check/executor.go"* ]] \
       || [[ "$line" == *"internal/check/summary.go"* ]] \
       || [[ "$line" == *"internal/embedding/"* ]] \
       || [[ "$line" == *"cmd/dev_progress.go"* ]] \
       || [[ "$line" == *"cmd/check.go"* ]]; then
        continue
    fi

    if [[ $line =~ ([0-9]+)[[:space:]]+([0-9]+)$ ]]; then
        stmts="${BASH_REMATCH[1]}"
        hits="${BASH_REMATCH[2]}"

        total_statements=$((total_statements + stmts))
        if [ "$hits" -gt 0 ]; then
            covered_statements=$((covered_statements + stmts))
        fi

        # Derive package from the file path (strip filename after last '/').
        # Line shape: "github.com/.../pkg/file.go:line.col,line.col N M"
        file_path="${line%%:*}"
        pkg="${file_path%/*}"
        idx=$(pkg_index "$pkg")
        if [ "$idx" -eq -1 ]; then
            PKG_ACC_KEYS+=("$pkg")
            PKG_ACC_TOTAL+=("$stmts")
            if [ "$hits" -gt 0 ]; then
                PKG_ACC_COVERED+=("$stmts")
            else
                PKG_ACC_COVERED+=(0)
            fi
        else
            PKG_ACC_TOTAL[$idx]=$((PKG_ACC_TOTAL[idx] + stmts))
            if [ "$hits" -gt 0 ]; then
                PKG_ACC_COVERED[$idx]=$((PKG_ACC_COVERED[idx] + stmts))
            fi
        fi
    fi
done < "$PERPKG_COV"

if [ "$total_statements" -eq 0 ]; then
    echo "❌ Failed to parse coverage data"
    exit 1
fi

total_coverage=$(echo "scale=2; $covered_statements * 100 / $total_statements" | bc -l)

# Compare coverage (using bc for floating point comparison)
if command -v bc &> /dev/null; then
    result=$(echo "$total_coverage >= $MIN_COVERAGE" | bc -l)
else
    # Fallback to awk if bc not available
    result=$(awk -v tc="$total_coverage" -v min="$MIN_COVERAGE" 'BEGIN {print (tc >= min)}')
fi

printf "📊 Project Coverage: %s%% (%d/%d statements)\n" "${total_coverage}" "${covered_statements}" "${total_statements}"
echo "🎯 Minimum Required: ${MIN_COVERAGE}%"

# Check per-package floors. Failures here mean a critical-tier package
# slipped below its ratchet — the regression is localized and loud by design.
tier_failed=0
tier_results=""
floor_count=${#PKG_FLOOR_KEYS[@]}
for (( i=0; i<floor_count; i++ )); do
    pkg="${PKG_FLOOR_KEYS[$i]}"
    floor="${PKG_FLOOR_VALUES[$i]}"
    idx=$(pkg_index "$pkg")

    if [ "$idx" -eq -1 ]; then
        tier_results="${tier_results}  ⚠️  $pkg: not found in coverage (required floor ${floor}%)\n"
        tier_failed=1
        continue
    fi

    total="${PKG_ACC_TOTAL[$idx]}"
    covered="${PKG_ACC_COVERED[$idx]}"

    pct=$(echo "scale=2; $covered * 100 / $total" | bc -l)
    if command -v bc &> /dev/null; then
        pass=$(echo "$pct >= $floor" | bc -l)
    else
        pass=$(awk -v p="$pct" -v f="$floor" 'BEGIN {print (p >= f)}')
    fi

    if [ "$pass" -eq 1 ]; then
        tier_results="${tier_results}  ✅ $pkg: ${pct}% (floor ${floor}%)\n"
    else
        tier_results="${tier_results}  ❌ $pkg: ${pct}% (floor ${floor}%)\n"
        tier_failed=1
    fi
done

echo ""
echo "🔒 Per-package floors (data-integrity spine / enforcement layer):"
printf "%b" "$tier_results"

if [ "$result" -eq 1 ] && [ "$tier_failed" -eq 0 ]; then
    echo ""
    echo "✅ Coverage check passed!"
    exit 0
elif [ "$tier_failed" -ne 0 ]; then
    echo ""
    echo "❌ Coverage check failed!"
    echo "   One or more critical packages fell below their ratchet floor."
    echo "   Raise coverage on the flagged package(s) — ratchets only move up."
    exit 1
else
    diff=$(awk -v tc="$total_coverage" -v min="$MIN_COVERAGE" 'BEGIN {printf "%.2f", min - tc}')
    echo ""
    echo "❌ Coverage check failed!"
    echo "   Need ${diff}% more coverage to reach ${MIN_COVERAGE}%"
    exit 1
fi
