#!/bin/bash
# Build the vaultmind binary with -tags ORT when lib/libtokenizers.a is
# present, falling back to a plain go build otherwise.
#
# Single source of truth for "how to rebuild vaultmind correctly" — used by:
#   - .claude/scripts/load-persona.sh (vaultmind SessionStart hook)
#   - workhorse's .claude/scripts/load-persona.sh (via $VAULTMIND_SRC)
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

if [ -f "$(pwd)/lib/libtokenizers.a" ]; then
    CGO_LDFLAGS="-L$(pwd)/lib" go build -tags ORT -o "$OUTPUT" .
else
    echo "[build-vaultmind] WARNING: lib/libtokenizers.a missing; building without -tags ORT." >&2
    echo "[build-vaultmind] BGE-M3 sparse/colbert indexing will fail on this binary." >&2
    echo "[build-vaultmind] To fix: run 'bash .claude/scripts/setup-ort.sh' from the vaultmind source dir." >&2
    go build -o "$OUTPUT" .
fi
