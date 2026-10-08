#!/bin/bash
# Verify the installed binary's ORT tag matches setup-ort.sh --check.
# A ready toolchain must produce an ORT binary; otherwise install uses pure-Go.
# Hugot v0.8.1's Go tokenizer does not require libtokenizers.a.
#
# Usage:
#   check-installed-ort-tag.sh [binary-path]
#
# Defaults: binary path = $(command -v vaultmind); project dir = $(pwd).
# Set PROJECT_DIR explicitly when invoking from outside the repo.
#
# Detection mechanism: ORT-only helpers (shouldEnableCoreML and
# detectORTLibDir from session_ort.go, which is gated by //go:build cgo &&
# ORT) appear in the symbol table iff the binary was built with -tags ORT.
# The pure-Go session_go.go is a different file with neither helper.

set -e

BINARY="${1:-$(command -v vaultmind 2>/dev/null || true)}"
PROJECT_DIR="${PROJECT_DIR:-$(pwd)}"

if [ -z "$BINARY" ] || [ ! -x "$BINARY" ]; then
    echo "[check-installed-ort-tag] no executable vaultmind found at: ${BINARY:-<empty>}" >&2
    exit 1
fi

if ! command -v go >/dev/null 2>&1; then
    echo "[check-installed-ort-tag] 'go' not on PATH — symbol inspection requires the Go toolchain" >&2
    exit 1
fi

TOOLCHAIN_READY=0
if (cd "$PROJECT_DIR" && bash .claude/scripts/setup-ort.sh --check > /dev/null 2>&1); then
    TOOLCHAIN_READY=1
fi

NM_OUTPUT=$(go tool nm "$BINARY" 2>&1) || {
    echo "[check-installed-ort-tag] go tool nm failed on $BINARY — binary may be stripped." >&2
    echo "  Symbol-based ORT-tag detection cannot run on stripped binaries. Aborting." >&2
    exit 1
}
HAS_ORT=0
if echo "$NM_OUTPUT" | grep -qE "embedding\.(shouldEnableCoreML|detectORTLibDir)"; then
    HAS_ORT=1
fi

if [ "$TOOLCHAIN_READY" -eq "$HAS_ORT" ]; then
    if [ "$HAS_ORT" -eq 1 ]; then
        echo "OK installed binary is ORT-tagged (matches ready ORT toolchain): $BINARY"
    else
        echo "OK installed binary is pure-Go (matches unavailable ORT toolchain): $BINARY"
    fi
    exit 0
fi

if [ "$TOOLCHAIN_READY" -eq 1 ]; then
    cat >&2 <<EOF
[check-installed-ort-tag] MISMATCH
  binary:           $BINARY
  ORT toolchain ready: yes
  binary ORT-tagged:           no

  Runtime check (BackendName) will report "go" and DefaultModel() will
  recommend MiniLM — silently degrading BGE-M3 vaults at the consumer
  surface. Rebuild with: task install
EOF
    exit 1
fi

cat >&2 <<EOF
[check-installed-ort-tag] MISMATCH
  binary:           $BINARY
  ORT toolchain ready: no
  binary ORT-tagged:           yes

  Binary is ORT-tagged but the toolchain is unavailable. Either
  restore with 'task setup:ort' or rebuild without ORT: task install
EOF
exit 1
