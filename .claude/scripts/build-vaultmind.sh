#!/bin/bash
# Build the vaultmind binary with -tags ORT when lib/libtokenizers.a is
# present, falling back to a plain go build otherwise.
#
# Single source of truth for "how to rebuild vaultmind correctly" — used by:
#   - .claude/scripts/load-persona.sh (vaultmind SessionStart hook)
#   - the companion project's .claude/scripts/load-persona.sh (via $VAULTMIND_SRC)
#   - Taskfile.yml's check:bootstrap step
#
# Without -tags ORT and the matching CGO_LDFLAGS, the resulting binary cannot
# index BGE-M3 sparse/colbert and silently degrades the hybrid retrieval
# stack. Each historical instance of "auto-rebuild produces pure-Go binary"
# (vaultmind#29 and the load-persona regressions before commit edad62e) was
# the same root cause expressed at a different call site. This script is the
# enforcement-by-design fix.
#
# Usage: build-vaultmind.sh [output_path]
#   output_path defaults to /tmp/vaultmind
#
# Must be invoked with the vaultmind source directory as the working
# directory (so $(pwd)/lib resolves to the project-local libtokenizers).

set -e

OUTPUT="${1:-/tmp/vaultmind}"

# Version-stamp the binary so `vaultmind --version` reports the real commit,
# not "dev". Mirrors the ldflags in Taskfile.yml's build:/install:/build:ort.
# Computed inline because this is standalone bash (no Task var context).
# Without it, hook-rebuilt / SSOT-rebuilt binaries reported `version dev`
# (TRIZ dogfood 2026-05-29). Values are space-free, so one -ldflags arg works.
MODULE_PATH="$(go list -m 2>/dev/null || echo github.com/peiman/vaultmind)"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u '+%Y-%m-%d_%H:%M:%S')"
LDFLAGS="-X ${MODULE_PATH}/cmd.binaryName=vaultmind -X ${MODULE_PATH}/cmd.Version=${VERSION} -X ${MODULE_PATH}/cmd.Commit=${COMMIT} -X ${MODULE_PATH}/cmd.Date=${DATE}"

if [ -f "$(pwd)/lib/libtokenizers.a" ]; then
    CGO_LDFLAGS="-L$(pwd)/lib" go build -tags ORT -ldflags="$LDFLAGS" -o "$OUTPUT" .
else
    echo "[build-vaultmind] WARNING: lib/libtokenizers.a missing; building without -tags ORT." >&2
    echo "[build-vaultmind] BGE-M3 sparse/colbert indexing will fail on this binary." >&2
    echo "[build-vaultmind] To fix: run 'bash .claude/scripts/setup-ort.sh' from the vaultmind source dir." >&2
    go build -ldflags="$LDFLAGS" -o "$OUTPUT" .
fi
