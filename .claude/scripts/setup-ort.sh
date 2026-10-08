#!/bin/bash
# Install the ORT build dependencies for BGE-M3 indexing.
#
# Usage: bash .claude/scripts/setup-ort.sh [--check | --tokenizers-version]
#
# Idempotent — safe to run multiple times. Steps:
#   1. Verify libonnxruntime.{dylib,so} is on the system (homebrew or /usr/local/lib)
#   2. Resolve hugot's tokenizer dependency and selected Go module version (SSOT)
#   3. For legacy daulet/tokenizers only, download the matching native release
#   4. Extract libtokenizers.a into project-local lib/
#   5. Print the CGO_LDFLAGS that `task build:ort` will use
#
# With --check, steps that WOULD modify state are skipped; only verification
# runs. Non-zero exit means `task build:ort` will not succeed.
#
# With --tokenizers-version, only resolve the tokenizer; no native libraries
# are probed or installed. Used by the cheap PR toolchain check.
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

# Resolve the dependency explicitly: hugot v0.8.1 uses go-huggingface's Go
# tokenizer for every backend. Older hugot versions used daulet's native one.
# Use the selected module version, which can be higher than hugot's requirement.
if ! HUGOT_GOMOD="$(go list -m -f '{{.GoMod}}' github.com/knights-analytics/hugot)" || [ ! -f "$HUGOT_GOMOD" ]; then
  echo "$FAIL hugot module not in cache: $HUGOT_GOMOD"
  echo "    run: go mod download github.com/knights-analytics/hugot"
  exit 1
fi
TOKENIZERS_MODULE="$(awk '
  $1 == "github.com/daulet/tokenizers" {native = $1}
  $1 == "github.com/gomlx/go-huggingface" {go_tokenizer = $1}
  END {print (native != "" ? native : go_tokenizer)}
' "$HUGOT_GOMOD")"
if [ -z "$TOKENIZERS_MODULE" ]; then
  echo "$FAIL could not identify tokenizer dependency in $HUGOT_GOMOD (expected daulet/tokenizers or gomlx/go-huggingface)"
  exit 1
fi
if ! TOKENIZERS_VERSION="$(go list -m -f '{{.Version}}' "$TOKENIZERS_MODULE")" || [ -z "$TOKENIZERS_VERSION" ]; then
  echo "$FAIL could not resolve tokenizer version for $TOKENIZERS_MODULE required by $HUGOT_GOMOD"
  exit 1
fi
if [ "$TOKENIZERS_MODULE" = "github.com/gomlx/go-huggingface" ]; then
  echo "$PASS $TOKENIZERS_MODULE $TOKENIZERS_VERSION (Go tokenizer; no libtokenizers.a required)"
else
  echo "$PASS $TOKENIZERS_MODULE $TOKENIZERS_VERSION"
fi
if [ "${1:-}" = "--tokenizers-version" ]; then
  exit 0
fi

# Minimum ONNX Runtime: microsoft/onnxruntime/go (pinned via hugot 0.8.1) requests
# ORT API 29, first shipped in ONNX Runtime 1.29. Keep in step with the bundled
# ORT_VERSION in .github/workflows/ci.yml.
ORT_MIN_VERSION="1.29.0"

# ort_version_in prints the version of the versioned libonnxruntime in $1
# (libonnxruntime.X.Y.Z.dylib or libonnxruntime.so.X.Y.Z), or nothing.
ort_version_in() {
  local f
  for f in "$1"/libonnxruntime.*.dylib "$1"/libonnxruntime.so.*; do
    [ -e "$f" ] || continue
    f="${f##*/}"
    f="${f#libonnxruntime.}"; f="${f#so.}"; f="${f%.dylib}"
    case "$f" in [0-9]*.[0-9]*.[0-9]*) echo "$f"; return ;; esac
  done
}

# version_lt succeeds when dotted version $1 < $2.
version_lt() {
  [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | head -n1)" = "$1" ]
}

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
  # The runtime must be new enough for the Go binding hugot pins. With an older
  # one the build succeeds and every BGE-M3 session then fails at startup:
  # "The requested API version [29] is not available ... ORT Version is:
  # 1.25.0" (#148). Checked here, where it is still a setup problem.
  ORT_FOUND_VERSION="$(ort_version_in "$ORT_LIB_DIR")"
  if [ -z "$ORT_FOUND_VERSION" ]; then
    echo "  ⚠ could not read the libonnxruntime version in $ORT_LIB_DIR — needs >= $ORT_MIN_VERSION"
  elif version_lt "$ORT_FOUND_VERSION" "$ORT_MIN_VERSION"; then
    echo "$FAIL libonnxruntime $ORT_FOUND_VERSION is too old — needs >= $ORT_MIN_VERSION"
    echo "    upgrade with: brew upgrade onnxruntime  (macOS)"
    echo "    or download from https://github.com/microsoft/onnxruntime/releases"
    FAILED=1
  else
    echo "$PASS libonnxruntime $ORT_FOUND_VERSION (>= $ORT_MIN_VERSION)"
  fi
fi

# --- Step 4: libtokenizers.a -----------------------------------------------

echo ""
echo "2. native tokenizer (legacy hugot only)"

needs_download=0
if [ "$TOKENIZERS_MODULE" = "github.com/gomlx/go-huggingface" ]; then
  echo "$PASS Go tokenizer — native tokenizer setup skipped"
elif [ ! -f "$LIBTOKENIZERS" ]; then
  needs_download=1
fi

if [ "$needs_download" = "1" ]; then
  if [ "$CHECK_ONLY" = "1" ]; then
    echo "$WARN $LIBTOKENIZERS missing; run without --check to install"
    FAILED=1
  else
    echo "   downloading libtokenizers.${PLATFORM}.tar.gz..."
    mkdir -p "$LIB_DIR"
    TOKENIZERS_TMP="$(mktemp -d)"
    trap 'rm -rf "$TOKENIZERS_TMP"' EXIT
    URL="https://github.com/daulet/tokenizers/releases/download/${TOKENIZERS_VERSION}/libtokenizers.${PLATFORM}.tar.gz"
    if ! curl -fsSL -o "$TOKENIZERS_TMP/lt.tar.gz" "$URL"; then
      echo "$FAIL download failed: $URL"
      exit 1
    fi
    tar -xzf "$TOKENIZERS_TMP/lt.tar.gz" -C "$TOKENIZERS_TMP"
    if [ ! -f "$TOKENIZERS_TMP/libtokenizers.a" ]; then
      echo "$FAIL archive did not contain libtokenizers.a"
      exit 1
    fi
    mv "$TOKENIZERS_TMP/libtokenizers.a" "$LIBTOKENIZERS"
    echo "$PASS installed $LIBTOKENIZERS ($(wc -c < "$LIBTOKENIZERS" | awk '{print int($1/1024/1024)"MB"}'))"
  fi
elif [ "$TOKENIZERS_MODULE" = "github.com/daulet/tokenizers" ]; then
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
