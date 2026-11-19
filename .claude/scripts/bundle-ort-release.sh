#!/usr/bin/env bash
# Build a self-contained prebuilt-ORT release archive: the `-tags ORT` vaultmind
# binary + a bundled, official Microsoft libonnxruntime. An adopter extracts
# and runs — full 4-way BGE-M3 hybrid (dense + sparse + ColBERT), with
# no source build and no system dependency.
#
# The binary dlopen's libonnxruntime at runtime, and detectORTLibDir checks the
# executable's OWN directory first, so the bundled sibling lib is found with zero
# config (no ORT_LIB_DIR needed). We bundle the OFFICIAL Microsoft lib — not the
# Homebrew one, which is a thin shim that drags in a tree of Homebrew dylibs and
# would not load on a clean machine.
#
# Usage:
#   bundle-ort-release.sh --goos darwin --goarch arm64 --ort-version 1.30.0 \
#       --ort-libdir ./ort/lib --binary ./vaultmind --version v0.1.1 --out dist
#
# Prints the produced archive path on stdout. All progress goes to stderr.
set -euo pipefail

GOOS=""; GOARCH=""; ORT_VERSION=""; BINARY=""; VERSION=""; PROJECT="vaultmind"; OUT="dist"; ORT_LIBDIR_IN=""
while [ $# -gt 0 ]; do
  case "$1" in
    --goos)        GOOS="$2";        shift 2 ;;
    --goarch)      GOARCH="$2";      shift 2 ;;
    --ort-version) ORT_VERSION="$2"; shift 2 ;;
    --binary)      BINARY="$2";      shift 2 ;;
    --version)     VERSION="$2";     shift 2 ;;
    --project)     PROJECT="$2";     shift 2 ;;
    --out)         OUT="$2";         shift 2 ;;
    --ort-libdir)  ORT_LIBDIR_IN="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[ -n "$GOOS" ] && [ -n "$GOARCH" ] && [ -n "$ORT_VERSION" ] && [ -n "$BINARY" ] && [ -n "$VERSION" ] || {
  echo "usage: bundle-ort-release.sh --goos <os> --goarch <arch> --ort-version <v> --ort-libdir <dir> --binary <path> --version <v> [--project p] [--out dir]" >&2
  exit 2
}
[ -n "$ORT_LIBDIR_IN" ] || {
  echo "error: --ort-libdir is required (provide the extracted ONNX Runtime library directory)" >&2
  exit 2
}
[ -f "$BINARY" ] || { echo "binary not found: $BINARY" >&2; exit 2; }

# Map platform -> the lib basename the
# binary looks for (detectORTLibDir checks for exactly this name).
case "${GOOS}/${GOARCH}" in
  darwin/arm64) LIBMAIN="libonnxruntime.dylib" ;;
  darwin/amd64) LIBMAIN="libonnxruntime.dylib" ;;
  linux/amd64)  LIBMAIN="libonnxruntime.so" ;;
  *) echo "unsupported platform ${GOOS}/${GOARCH}" >&2; exit 2 ;;
esac

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ORT_LIBDIR="$ORT_LIBDIR_IN"
echo "• using provided ONNX Runtime lib dir: ${ORT_LIBDIR}" >&2
[ -d "$ORT_LIBDIR" ] && ls "$ORT_LIBDIR"/libonnxruntime* >/dev/null 2>&1 || {
  echo "no libonnxruntime found in ${ORT_LIBDIR}" >&2; exit 1
}

STAGE_NAME="${PROJECT}_${VERSION}_${GOOS}_${GOARCH}_ort"
STAGE="$WORK/$STAGE_NAME"
mkdir -p "$STAGE"

# The binary (named for the project, executable).
cp "$BINARY" "$STAGE/${PROJECT}"
chmod +x "$STAGE/${PROJECT}"

# The official lib(s), preserving the distribution's own symlink layout (cp -a).
# Exclude *.dSYM — debug symbols, not needed at runtime.
find "$ORT_LIBDIR" -maxdepth 1 -name 'libonnxruntime*' ! -name '*.dSYM' -exec cp -a {} "$STAGE/" \;
# Guarantee the exact unversioned basename the binary looks for exists.
if [ ! -e "$STAGE/${LIBMAIN}" ]; then
  REAL="$(find "$STAGE" -maxdepth 1 -name "libonnxruntime*" -type f | head -1)"
  [ -n "$REAL" ] && ln -s "$(basename "$REAL")" "$STAGE/${LIBMAIN}"
fi
[ -e "$STAGE/${LIBMAIN}" ] || { echo "could not provide ${LIBMAIN} in bundle" >&2; exit 1; }

# Docs.
[ -f LICENSE ]   && cp LICENSE   "$STAGE/" || true
[ -f README.md ] && cp README.md "$STAGE/" || true
cat > "$STAGE/README-ORT.txt" <<EOF
vaultmind — prebuilt ORT (BGE-M3) build for ${GOOS}/${GOARCH}, ${VERSION}

This is the FULL 4-way BGE-M3 hybrid build (dense + sparse + ColBERT), with a
bundled, self-contained libonnxruntime (official Microsoft ONNX Runtime
${ORT_VERSION}). No source build, no system dependency.

Run it:
  ./${PROJECT} --version
  ./${PROJECT} doctor --vault <your-vault>

The binary finds the bundled libonnxruntime automatically (it checks its own
directory). If you move the binary, keep the libonnxruntime file beside it, or
set ORT_LIB_DIR to the directory that holds it.

ONNX Runtime is MIT-licensed by Microsoft (https://www.nuget.org/packages/Microsoft.ML.OnnxRuntime).
vaultmind's license is in LICENSE.
EOF

mkdir -p "$OUT"
ARCHIVE="${OUT}/${STAGE_NAME}.tar.gz"
tar czf "$ARCHIVE" -C "$WORK" "$STAGE_NAME"
echo "✓ ${ARCHIVE} ($(du -h "$ARCHIVE" | cut -f1))" >&2
echo "$ARCHIVE"
