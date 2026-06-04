#!/bin/bash
# Install the ORT build dependencies for BGE-M3 indexing.
#
# Usage: bash .claude/scripts/setup-ort.sh [--check]
#
# Idempotent — safe to run multiple times. Steps:
#   1. Verify libonnxruntime.{dylib,so} is on the system (homebrew or /usr/local/lib)
#   2. Read pinned daulet/tokenizers version from hugot's go.mod (SSOT)
#   3. Download matching libtokenizers release for this OS/arch
#   4. Extract libtokenizers.a into project-local lib/
#   5. Print the CGO_LDFLAGS that `task build:ort` will use
#
# With --check, steps that WOULD modify state are skipped; only verification
# runs. Non-zero exit means `task build:ort` will not succeed.
#
# Why project-local? libtokenizers.a is a single static lib; shipping it in
# lib/ keeps the ORT build hermetic, avoids system-wide install, and keeps
# the dependency visible in the repo. Opt-in via -tags ORT; not required for
# the default pure-Go build.

set -e

CHECK_ONLY=0
if [ "${1:-}" = "--check" ]; then
  CHECK_ONLY=1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
LIB_DIR="$PROJECT_DIR/lib"
LIBTOKENIZERS="$LIB_DIR/libtokenizers.a"

PASS="  ✓"
WARN="  ⚠"
FAIL="  ✗"
FAILED=0

echo "ORT build setup for $PROJECT_DIR"
echo ""

# --- Step 1: platform detection -------------------------------------------

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS/$ARCH" in
  Darwin/arm64)    PLATFORM="darwin-arm64";    DYLIB="libonnxruntime.dylib" ;;
  Darwin/x86_64)   PLATFORM="darwin-x86_64";   DYLIB="libonnxruntime.dylib" ;;
  Linux/x86_64)    PLATFORM="linux-amd64";     DYLIB="libonnxruntime.so"    ;;
  Linux/aarch64)   PLATFORM="linux-aarch64";   DYLIB="libonnxruntime.so"    ;;
  Linux/arm64)     PLATFORM="linux-arm64";     DYLIB="libonnxruntime.so"    ;;
  *)
    echo "$FAIL unsupported platform: $OS/$ARCH"
    exit 1
    ;;
esac
echo "$PASS platform: $PLATFORM"

# --- Step 2: libonnxruntime ------------------------------------------------

echo ""
echo "1. libonnxruntime (system)"
ORT_LIB_DIR=""
for dir in /opt/homebrew/lib /usr/local/lib /usr/lib; do
  if [ -f "$dir/$DYLIB" ]; then
    ORT_LIB_DIR="$dir"
    break
  fi
done

if [ -z "$ORT_LIB_DIR" ]; then
  echo "$FAIL $DYLIB not found on this system"
  echo "    install with: brew install onnxruntime  (macOS)"
  echo "    or download from https://github.com/microsoft/onnxruntime/releases"
  FAILED=1
else
  echo "$PASS $ORT_LIB_DIR/$DYLIB"
fi

# --- Step 3: libtokenizers version from hugot's go.mod ---------------------

echo ""
echo "2. libtokenizers version (pinned via hugot)"
HUGOT_GOMOD="$(go env GOMODCACHE)/github.com/knights-analytics/hugot@$(
  go list -m -f '{{.Version}}' github.com/knights-analytics/hugot
)/go.mod"

if [ ! -f "$HUGOT_GOMOD" ]; then
  echo "$FAIL hugot module not in cache: $HUGOT_GOMOD"
  echo "    run: go mod download github.com/knights-analytics/hugot"
  exit 1
fi

TOKENIZERS_VERSION="$(
  grep 'github.com/daulet/tokenizers' "$HUGOT_GOMOD" | awk '{print $2}' | tr -d '\r'
)"
if [ -z "$TOKENIZERS_VERSION" ]; then
  echo "$FAIL could not read daulet/tokenizers version from $HUGOT_GOMOD"
  exit 1
fi
echo "$PASS daulet/tokenizers $TOKENIZERS_VERSION"

# --- Step 4: libtokenizers.a -----------------------------------------------

echo ""
echo "3. libtokenizers.a (project-local)"

needs_download=0
if [ ! -f "$LIBTOKENIZERS" ]; then
  needs_download=1
fi

if [ "$needs_download" = "1" ]; then
  if [ "$CHECK_ONLY" = "1" ]; then
    echo "$WARN $LIBTOKENIZERS missing; run without --check to install"
    FAILED=1
  else
    echo "   downloading libtokenizers.${PLATFORM}.tar.gz..."
    mkdir -p "$LIB_DIR"
    TMPDIR="$(mktemp -d)"
    trap 'rm -rf "$TMPDIR"' EXIT
    URL="https://github.com/daulet/tokenizers/releases/download/${TOKENIZERS_VERSION}/libtokenizers.${PLATFORM}.tar.gz"
    if ! curl -fsSL -o "$TMPDIR/lt.tar.gz" "$URL"; then
      echo "$FAIL download failed: $URL"
      exit 1
    fi
    tar -xzf "$TMPDIR/lt.tar.gz" -C "$TMPDIR"
    if [ ! -f "$TMPDIR/libtokenizers.a" ]; then
      echo "$FAIL archive did not contain libtokenizers.a"
      exit 1
    fi
    mv "$TMPDIR/libtokenizers.a" "$LIBTOKENIZERS"
    echo "$PASS installed $LIBTOKENIZERS ($(wc -c < "$LIBTOKENIZERS" | awk '{print int($1/1024/1024)"MB"}'))"
  fi
else
  echo "$PASS $LIBTOKENIZERS already present"
fi

# --- Step 5: summary -------------------------------------------------------

echo ""
if [ "$FAILED" = "1" ]; then
  echo "$FAIL ORT setup incomplete"
  exit 1
fi
echo "$PASS ORT setup ready"
echo ""
echo "To build: task build:ort"
echo "CGO_LDFLAGS used: -L$LIB_DIR"
