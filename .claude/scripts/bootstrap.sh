#!/bin/bash
# Bootstrap VaultMind for the current user.
#
# Usage: bash .claude/scripts/bootstrap.sh [--check]
#
# Idempotent — safe to run multiple times. Steps:
#   1. Build /tmp/vaultmind from source
#   2. Embed every vault directory in this project (vaultmind-identity,
#      vaultmind-vault, any other vault-*)
#   3. Verify the SessionStart persona hook is wired in settings.json
#   4. Smoke-test the hook by running load-persona.sh
#
# With --check, steps that WOULD modify state are skipped; only verification
# runs. Non-zero exit means the setup needs work.

set -e

CHECK_ONLY=0
if [ "${1:-}" = "--check" ]; then
  CHECK_ONLY=1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
VAULTMIND="/tmp/vaultmind"

PASS="  ✓"
WARN="  ⚠"
FAIL="  ✗"
FAILED=0

echo "VaultMind bootstrap for $PROJECT_DIR"
echo ""

# --- Step 1: binary ---------------------------------------------------------

echo "1. VaultMind binary"
needs_build=0
if [ ! -f "$VAULTMIND" ]; then
  needs_build=1
elif [ -n "$(find "$PROJECT_DIR" -name '*.go' -newer "$VAULTMIND" -print -quit 2>/dev/null)" ]; then
  needs_build=1
fi

if [ "$needs_build" = "1" ]; then
  if [ "$CHECK_ONLY" = "1" ]; then
    echo "$WARN binary is stale; run without --check to rebuild"
    FAILED=1
  else
    echo "   building..."
    (cd "$PROJECT_DIR" && go build -o "$VAULTMIND" .)
    echo "$PASS built $VAULTMIND"
  fi
else
  echo "$PASS binary is current: $VAULTMIND"
fi

# --- Step 2: vaults ---------------------------------------------------------

echo ""
echo "2. Vaults"
any_vault=0
for vault in "$PROJECT_DIR"/vaultmind-identity "$PROJECT_DIR"/vaultmind-vault; do
  if [ ! -d "$vault" ]; then continue; fi
  any_vault=1
  name=$(basename "$vault")

  if [ ! -f "$VAULTMIND" ]; then
    echo "$WARN $name: cannot check — binary missing"
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
done
if [ "$any_vault" = "0" ]; then
  echo "$WARN no vault directories found under $PROJECT_DIR"
fi

# --- Step 3: hook wiring ----------------------------------------------------

echo ""
echo "3. SessionStart hook wiring"
settings="$PROJECT_DIR/.claude/settings.json"
if [ ! -f "$settings" ]; then
  echo "$FAIL $settings missing"
  FAILED=1
elif grep -q "load-persona.sh" "$settings"; then
  echo "$PASS load-persona.sh is wired in settings.json"
else
  echo "$FAIL load-persona.sh script exists but is NOT wired in settings.json"
  echo "     add a SessionStart hook entry pointing at .claude/scripts/load-persona.sh"
  FAILED=1
fi

# --- Step 4: smoke test -----------------------------------------------------

echo ""
echo "4. Hook smoke test"
if [ "$CHECK_ONLY" = "1" ]; then
  echo "   skipped (--check)"
elif [ ! -f "$PROJECT_DIR/.claude/scripts/load-persona.sh" ]; then
  echo "$FAIL load-persona.sh not found"
  FAILED=1
else
  output=$(echo '{"session_id":"bootstrap-smoke-test"}' | bash "$PROJECT_DIR/.claude/scripts/load-persona.sh" 2>&1 || true)
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
