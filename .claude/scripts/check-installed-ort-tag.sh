#!/bin/bash
# Verify the installed vaultmind binary's ORT linkage matches the project's
# lib/libtokenizers.a presence.
#
# The contract enforced here: at install time, lib/libtokenizers.a presence
# MUST produce an ORT-tagged binary; its absence MUST produce a pure-Go
# binary. When the two disagree, the runtime-aware DefaultModel() picks the
# wrong default at the consumer surface — e.g. a BGE-M3 vault gets reindexed
# with MiniLM silently because BackendName() reports "go" even though
# libtokenizers is sitting in lib/. The companion project caught this on 2026-05-27;
# fix is structural — install must honor build's smart-default.
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

HAS_LIBTOK=0
[ -f "$PROJECT_DIR/lib/libtokenizers.a" ] && HAS_LIBTOK=1

NM_OUTPUT=$(go tool nm "$BINARY" 2>&1) || {
    echo "[check-installed-ort-tag] go tool nm failed on $BINARY — binary may be stripped." >&2
    echo "  Symbol-based ORT-tag detection cannot run on stripped binaries. Aborting." >&2
    exit 1
}
HAS_ORT=0
if echo "$NM_OUTPUT" | grep -qE "embedding\.(shouldEnableCoreML|detectORTLibDir)"; then
    HAS_ORT=1
fi

if [ "$HAS_LIBTOK" -eq "$HAS_ORT" ]; then
    if [ "$HAS_ORT" -eq 1 ]; then
        echo "OK installed binary is ORT-tagged (matches lib/libtokenizers.a presence): $BINARY"
    else
        echo "OK installed binary is pure-Go (matches lib/libtokenizers.a absence): $BINARY"
    fi
    exit 0
fi

if [ "$HAS_LIBTOK" -eq 1 ]; then
    cat >&2 <<EOF
[check-installed-ort-tag] MISMATCH
  binary:           $BINARY
  lib/libtokenizers.a present: yes
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
  lib/libtokenizers.a present: no
  binary ORT-tagged:           yes

  Binary expects libtokenizers but the project doesn't have it. Either
  restore with 'task setup:ort' or rebuild without ORT: task install
EOF
exit 1
