#!/bin/bash
# Vault-agnostic SessionStart health / onboarding nudge for VaultMind.
#
# Tells the agent (and human) the single next command for the current state:
#   no vault          -> silent (this hook only speaks when there's a vault)
#   vault, no binary  -> how to install
#   binary, no index  -> how to build the index
#   index on MiniLM   -> recall active but degraded; how to upgrade
#   index on BGE-M3   -> active; how to ask
#
# Why this exists: a project that ships a committed vaultmind vault but whose
# adopter hasn't installed the binary previously got SILENCE at session start —
# the recall/read hooks exit quietly when the binary is absent, and the only
# loud surface (load-persona.sh) is persona-shaped and writes to stderr (which
# SessionStart does NOT surface to the agent). This hook closes that gap for
# every adopter — persona and knowledge-vault alike. (focalc field report, P0.)
#
# Contract:
#   - Writes to STDOUT — SessionStart surfaces stdout to the agent, not stderr.
#   - Always exits 0, so a broken/empty/half-built vault can never wedge a
#     session start.
#   - The vaultmind binary is the single source of truth for the index tier;
#     this hook only relays `vaultmind doctor`, so it can never disagree with
#     the tool about whether the index is BGE-M3, MiniLM, or unbuilt.

# Drain the hook's stdin JSON (Claude Code passes session info); unused here.
cat >/dev/null 2>&1

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"

DOC="https://github.com/peiman/vaultmind/blob/main/docs/embedding-backends.md"

# Resolve the vault: explicit override wins, then conventional directory names.
VAULT="${VAULTMIND_VAULT:-}"
if [ -z "$VAULT" ]; then
  for cand in vaultmind-vault vaultmind-identity; do
    if [ -d "$PROJECT_DIR/$cand" ]; then
      VAULT="$PROJECT_DIR/$cand"
      break
    fi
  done
fi
# No vault -> stay silent. Nothing to onboard.
[ -n "$VAULT" ] && [ -d "$VAULT" ] || exit 0

# Resolve the binary: PATH first (go install / task install). The dev-loop
# build at /tmp/vaultmind is used ONLY inside a vaultmind source checkout, so an
# adopter's machine never picks up a stray /tmp binary. A missing binary is a
# first-class state, not an error.
if command -v vaultmind >/dev/null 2>&1; then
  VM="$(command -v vaultmind)"
elif [ -d "$PROJECT_DIR/internal" ] && [ -d "$PROJECT_DIR/cmd" ] && [ -x /tmp/vaultmind ]; then
  VM="/tmp/vaultmind"
else
  VM=""
fi

if [ -z "$VM" ]; then
  echo "📚 VaultMind vault detected ($VAULT) but the vaultmind binary is not installed."
  echo "   Install (MiniLM, any platform): go install github.com/peiman/vaultmind@latest   # Go >= 1.26.4"
  echo "   Full BGE-M3 hybrid (darwin-arm64/linux-amd64): download the prebuilt ORT archive from the release — no build. Other platforms: build from source — see $DOC"
  exit 0
fi

# Tier comes from the tool, never re-derived here (single source of truth).
# Capture doctor's exit code AND stderr: a doctor *failure* (bad vault path,
# corrupt/locked index, panicking or mismatched binary) must surface its reason,
# NOT be laundered into a success-shaped nudge. Suppressing the failure reason
# would reintroduce the exact silence this hook exists to end. Always exit 0.
ERRFILE="$(mktemp 2>/dev/null || echo "${TMPDIR:-/tmp}/vm-health-$$.err")"
DOCTOR="$("$VM" doctor --vault "$VAULT" 2>"$ERRFILE")"
DOCTOR_RC=$?
DOCTOR_ERR="$(cat "$ERRFILE" 2>/dev/null)"
rm -f "$ERRFILE"

if [ "$DOCTOR_RC" -ne 0 ]; then
  echo "⚠ VaultMind vault detected ($VAULT) but \`vaultmind doctor\` failed (exit $DOCTOR_RC)."
  [ -n "$DOCTOR_ERR" ] && echo "   ${DOCTOR_ERR}"
  echo "   Re-run to debug: vaultmind doctor --vault \"$VAULT\""
elif printf '%s' "$DOCTOR" | grep -q "Embeddings: none"; then
  echo "📚 VaultMind vault detected ($VAULT) — index not built yet."
  echo "   Build it: vaultmind index --vault \"$VAULT\" && vaultmind index --embed --vault \"$VAULT\""
elif printf '%s' "$DOCTOR" | grep -qE "\(minilm\)|degraded recall"; then
  echo "✓ VaultMind active ($VAULT) — but recall is degraded (MiniLM, 2-lane)."
  echo "   Ask: vaultmind ask \"<question>\" --vault \"$VAULT\""
  echo "   Full 4-way BGE-M3 hybrid: on darwin-arm64/linux-amd64 download the prebuilt ORT archive from the release (no build); other platforms see $DOC"
elif printf '%s' "$DOCTOR" | grep -qE "\(mixed\)|Partial BGE-M3 coverage"; then
  # A mixed-model index has a MiniLM fraction running degraded 2-lane recall —
  # the same cliff as a pure-MiniLM index, just partial. Name the full-embed fix.
  echo "✓ VaultMind active ($VAULT) — but recall is partially degraded (mixed-model index)."
  echo "   Finish the upgrade: vaultmind index --embed --vault \"$VAULT\""
  echo "   See $DOC"
elif printf '%s' "$DOCTOR" | grep -q "(bge-m3)"; then
  echo "✓ VaultMind active ($VAULT) — full BGE-M3 hybrid recall."
  echo "   Ask: vaultmind ask \"<question>\" --vault \"$VAULT\"  ->  vaultmind note get <id> --vault \"$VAULT\""
else
  # doctor SUCCEEDED (exit 0) but printed an unrecognized shape — e.g. an older
  # binary whose output predates the (minilm)/(bge-m3) tags. Keep the nudge
  # minimal and honest rather than guessing the tier.
  echo "📚 VaultMind vault detected ($VAULT). Try: vaultmind ask \"<question>\" --vault \"$VAULT\""
fi
exit 0
