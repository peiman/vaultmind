#!/usr/bin/env bash
# Fetch only the pinned BGE-M3 tokenizer.json into the legacy model cache
# the tests read when the XDG app name is unset:
# $HOME/.vaultmind/models/BAAI_bge-m3/tokenizer.json
# Does not download model weights. The sha256 is the pin in
# internal/embedding/download.go (bgem3Files). A Go test fails if they differ.
set -euo pipefail

revision=5617a9f61b028005a4858fdac845db406aefb181
sha=21106b6d7dab2952c1d496fb21d5dc9db75c28ed361a05f5020bbba27810dd08
url="https://huggingface.co/BAAI/bge-m3/resolve/${revision}/tokenizer.json"
dest="${HOME}/.vaultmind/models/BAAI_bge-m3/tokenizer.json"

verify() {
  local got
  if command -v sha256sum >/dev/null 2>&1; then
    got="$(sha256sum "$dest" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    got="$(shasum -a 256 "$dest" | awk '{print $1}')"
  else
    echo "sha256sum or shasum is required to verify ${dest}" >&2
    return 1
  fi
  if [ "$got" != "$sha" ]; then
    echo "tokenizer.json sha256 mismatch: got ${got} want ${sha}" >&2
    return 1
  fi
}

mkdir -p "$(dirname "$dest")"
# A failing verify in this condition does not trip set -e, so a cached file
# with the wrong hash is fetched again.
if [ -f "$dest" ] && verify; then
  echo "tokenizer.json already matches ${sha}"
  exit 0
fi

tmp="${dest}.partial"
curl -fsSL "$url" -o "$tmp"
mv "$tmp" "$dest"
verify
echo "tokenizer.json verified ${sha}"
