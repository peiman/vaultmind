#!/bin/bash
# Bootstrap VaultMind for the current user.
#
# Usage: bash .claude/scripts/bootstrap.sh [--check]
#
# Idempotent — safe to run multiple times. Steps:
#   1. Verify `vaultmind` is installed on PATH (run `task install` if not)
#   2. Embed the example vault (examples/ada-vault) and any vaultmind-* vault
#      present in this project
#   3. Verify the SessionStart persona hook is wired in settings.json
#   4. Smoke-test the hook by running load-persona.sh
#
# With --check, steps that WOULD modify state are skipped; only verification
# runs. Non-zero exit means the setup needs work.
#
# Note on /tmp/vaultmind: the dev-loop binary that load-persona.sh
# auto-rebuilds in-session lives at /tmp/vaultmind. This script does NOT
# pre-build it; that's load-persona.sh's responsibility. Bootstrap is the
# install-and-verify gate, not the dev-loop builder.

set -e

CHECK_ONLY=0
if [ "${1:-}" = "--check" ]; then
  CHECK_ONLY=1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS="  ✓"
WARN="  ⚠"
FAIL="  ✗"
FAILED=0

echo "VaultMind bootstrap for $PROJECT_DIR"
echo ""

# --- CI short-circuit -------------------------------------------------------
#
# This is a DEVELOPER-setup verification: a PATH-installed binary, embedded
# vaults (index.db is gitignored), and a wired SessionStart hook
# (settings.local.json is gitignored). None of that exists in a fresh CI
# checkout, by design — so in CI every step would "fail" on legitimately-absent
# local state. The thing CI can validate — that the binary BUILDS — already ran
# in the build step that precedes this in the `check:bootstrap` task. So in CI,
# --check is a no-op. (GitHub Actions and most CI set CI=true.)
if [ "$CHECK_ONLY" = "1" ] && [ -n "${CI:-}" ]; then
  echo "CI detected — skipping dev-environment verification (binary build validated separately)."
  echo "✅ VaultMind build is ready."
  exit 0
fi

# --- Step 1: PATH installation ---------------------------------------------
#
# `task install` produces a properly ldflags-stamped binary at
# $GOPATH/bin/vaultmind. That's the canonical binary for every context
# except the in-session dev loop (load-persona.sh's auto-rebuild to
# /tmp/vaultmind). Bootstrap exists to verify this install is current
# and matches the source the user just checked out — if you cloned
# vaultmind for the first time, this is the step that produces a
# working binary.

echo "1. VaultMind installation (PATH)"
needs_install=0
if ! command -v vaultmind >/dev/null 2>&1; then
  needs_install=1
fi

if [ "$needs_install" = "1" ]; then
  if [ "$CHECK_ONLY" = "1" ]; then
    echo "$WARN vaultmind not on PATH — run 'task install' to install"
    FAILED=1
  else
    echo "   installing via 'task install'..."
    (cd "$PROJECT_DIR" && task install >/dev/null 2>&1) || {
      echo "$FAIL task install failed; check 'task install' output manually"
      FAILED=1
    }
    if command -v vaultmind >/dev/null 2>&1; then
      echo "$PASS vaultmind installed at $(command -v vaultmind)"
    else
      echo "$WARN install ran but vaultmind still not on PATH"
      echo "      check that \$GOPATH/bin (or \$GOBIN) is in your PATH"
      FAILED=1
    fi
  fi
else
  PATH_VAULTMIND=$(command -v vaultmind)
  echo "$PASS vaultmind on PATH: $PATH_VAULTMIND"
  # Compare PATH binary version vs current source HEAD. If the user
  # has new commits since the last `task install`, the PATH binary
  # is behind. Soft warn, not a fail — they may have legitimate
  # reasons (testing rollback, comparing versions).
  if [ -x "$PATH_VAULTMIND" ]; then
    HEAD_COMMIT=$(cd "$PROJECT_DIR" && git rev-parse HEAD 2>/dev/null || echo "")
    PATH_COMMIT=$("$PATH_VAULTMIND" --version 2>/dev/null | grep -o "commit [a-f0-9]*" | awk '{print $2}' || echo "")
    if [ -n "$HEAD_COMMIT" ] && [ -n "$PATH_COMMIT" ] && [ "${HEAD_COMMIT:0:7}" != "${PATH_COMMIT:0:7}" ]; then
      echo "$WARN PATH binary is behind source HEAD"
      echo "      PATH:   ${PATH_COMMIT:0:7}"
      echo "      source: ${HEAD_COMMIT:0:7}"
      echo "      run 'task install' to refresh"
    fi
  fi
fi

# Resolve the binary used by subsequent steps. Use PATH-installed —
# /tmp/vaultmind is dev-loop only, owned by load-persona.sh.
VAULTMIND=$(command -v vaultmind 2>/dev/null || true)

# --- Step 2: vaults ---------------------------------------------------------

echo ""
echo "2. Vaults"
any_vault=0
for vault in "$PROJECT_DIR"/examples/ada-vault "$PROJECT_DIR"/vaultmind-*; do
  if [ ! -d "$vault" ]; then continue; fi
  any_vault=1
  name=$(basename "$vault")

  if [ -z "$VAULTMIND" ] || [ ! -x "$VAULTMIND" ]; then
    echo "$WARN $name: cannot check — vaultmind not on PATH"
    FAILED=1
    continue
  fi

  db="$vault/.vaultmind/index.db"
  if [ -f "$db" ]; then
    notes=$(sqlite3 "$db" "SELECT COUNT(*) FROM notes" 2>/dev/null || echo "0")
    embedded=$(sqlite3 "$db" "SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL" 2>/dev/null || echo "0")
  else
    notes=0
    embedded=0
  fi

  if [ "$notes" = "0" ] || [ "$embedded" != "$notes" ]; then
    if [ "$CHECK_ONLY" = "1" ]; then
      echo "$WARN $name: $embedded/$notes embedded; run without --check to fix"
      FAILED=1
    else
      echo "   embedding $name ($notes notes, $embedded currently embedded)..."
      "$VAULTMIND" index --embed --model minilm --vault "$vault" >/dev/null
      embedded=$(sqlite3 "$db" "SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL" 2>/dev/null || echo "?")
      notes=$(sqlite3 "$db" "SELECT COUNT(*) FROM notes" 2>/dev/null || echo "?")
      echo "$PASS $name: $embedded/$notes embedded"
    fi
  else
    echo "$PASS $name: $embedded/$notes embedded"
  fi

  # Dense-count parity is necessary but not sufficient. Under BGE-M3, hybrid
  # RRF ranking needs sparse and colbert in lockstep with dense; a partially-
  # covered vault silently compresses ranking (see the 2026-04-24 incident).
  # Delegate detection to `vaultmind doctor` so this script and the CLI share
  # a single source of truth. Loud warning, non-blocking — imbalance doesn't
  # stop other work; it just must not be invisible.
  if [ -f "$db" ] && "$VAULTMIND" doctor --vault "$vault" 2>/dev/null \
       | grep -q "Partial BGE-M3 coverage"; then
    echo "$WARN $name: BGE-M3 modality imbalance — run:"
    echo "     $VAULTMIND index --embed --model bge-m3 --vault $vault"
  fi
done
if [ "$any_vault" = "0" ]; then
  echo "$WARN no vault directories found under $PROJECT_DIR"
fi

# --- Step 3: hook wiring ----------------------------------------------------

echo ""
echo "3. SessionStart hook wiring"
settings="$PROJECT_DIR/.claude/settings.json"
settings_local="$PROJECT_DIR/.claude/settings.local.json"
hook_script="$PROJECT_DIR/internal/hookscripts/load-persona.sh"
if [ ! -f "$settings" ]; then
  echo "$FAIL $settings missing"
  FAILED=1
elif grep -q "hookscripts/load-persona.sh\|scripts/load-persona.sh" "$settings" 2>/dev/null; then
  echo "$PASS load-persona.sh is wired in settings.json"
elif [ -f "$settings_local" ] && grep -q "hookscripts/load-persona.sh\|scripts/load-persona.sh" "$settings_local" 2>/dev/null; then
  echo "$PASS load-persona.sh is wired in settings.local.json"
else
  echo "$FAIL load-persona.sh script exists but is NOT wired in either settings.json or settings.local.json"
  echo "     add a SessionStart hook entry pointing at internal/hookscripts/load-persona.sh"
  FAILED=1
fi

# --- Step 4: smoke test -----------------------------------------------------

echo ""
echo "4. Hook smoke test"
if [ "$CHECK_ONLY" = "1" ]; then
  echo "   skipped (--check)"
elif [ ! -f "$hook_script" ]; then
  echo "$FAIL load-persona.sh not found at $hook_script"
  FAILED=1
else
  output=$(echo '{"session_id":"bootstrap-smoke-test"}' | bash "$hook_script" 2>&1 || true)
  if echo "$output" | grep -q "IDENTITY CONTEXT"; then
    echo "$PASS hook runs and produces persona output"
  else
    echo "$FAIL hook ran but did not produce identity context"
    echo "     output: $output" | head -5
    FAILED=1
  fi
fi

# --- summary ----------------------------------------------------------------

echo ""
if [ "$FAILED" = "0" ]; then
  echo "✅ VaultMind is ready."
  exit 0
fi
echo "❌ VaultMind setup has issues. Re-run without --check to fix what can be fixed automatically."
exit 1
