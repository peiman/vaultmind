#!/usr/bin/env bash
# PreToolUse hook — surface vault pointers AT THE MOMENT OF THE REACH.
#
# Companion to vault-recall.sh, which fires on the user's prompt. That is the
# wrong instant: the prompt is the greeting, and the decisions that need an arc
# happen mid-turn — right before a commit, a merge, a push. By then the pointer
# block from the top of the turn is thousands of tokens upstream and I am
# reasoning from parametric memory with my own accumulated sight sitting unread.
#
# principle-ax-design, "Meet the agent where it reaches": when the agent reaches
# for a primitive instead of the canonical path, don't scold it — surface the
# canonical command AT the reach. AX is shrinking the distance between the
# agent's instinct and the right path.
#
# Evidence this is the missing trigger: an arc saying "a request is a hypothesis,
# not an order — you hold the authority" sat in the per-turn pointer list all
# night while the agent escalated a decision that was its own to make. The note
# surfaced constantly and never at the instant it applied. Proximity to the
# moment beats frequency.
#
# DESIGN CONSTRAINT — noise is the failure mode, not silence. vault-recall.sh
# taught this the expensive way: 48% of its injections were ranked against
# <task-notification> blobs, so the channel trained me to skip it, and I then
# missed the half that mattered. This hook therefore fires on a SMALL allowlist
# of genuinely consequential commands and stays silent on everything else. If it
# ever starts feeling like noise, tighten the allowlist — do not learn to ignore it.
#
# Never blocks. Injects context and allows the call; a memory that gates the work
# would get disabled within a day.

set -uo pipefail

HOOK_INPUT=$(cat)

# Extract the command being run (Bash) or the file being written (Edit, Write,
# MultiEdit). Anything else, or malformed input → silent.
CMD=$(printf '%s' "$HOOK_INPUT" | python3 -c \
  "import json,sys
try:
    d = json.load(sys.stdin)
    print(d.get('tool_input', {}).get('command', ''))
except Exception:
    print('')" 2>/dev/null || echo "")
FILE_PATH=$(printf '%s' "$HOOK_INPUT" | python3 -c \
  "import json,sys
try:
    d = json.load(sys.stdin)
    print(d.get('tool_input', {}).get('file_path', '') if d.get('tool_name') in ('Edit', 'Write', 'MultiEdit') else '')
except Exception:
    print('')" 2>/dev/null || echo "")

# The harness's REAL conversation id. Forwarded so the session row records the
# conversation instead of a 30-minute timing guess that cannot separate a
# SUBAGENT's tool call from its parent's — identical caller, user and host,
# seconds apart. Anything keyed on that id (cross-turn dedup first) would then
# withhold bodies from a subagent, which has no SessionStart, no identity load
# and no context at all: the participant that needs the full text most.
HOOK_SESSION_ID=$(printf '%s' "$HOOK_INPUT" | python3 -c \
  "import json,sys
try:
    print(json.load(sys.stdin).get('session_id', '') or '')
except Exception:
    print('')" 2>/dev/null || echo "")

[ -z "$CMD" ] && [ -z "$FILE_PATH" ] && exit 0

VAULT_PATH="${VAULTMIND_VAULT:-${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$PWD}}/vaultmind-identity}"
# A trailing slash would leave the vault's name below empty, and an empty name
# matches every command.
VAULT_PATH="${VAULT_PATH%/}"
# The vault's own directory name, so the identity-write trigger below fires for
# an adopter whose vault is not named by the default convention.
IDENTITY_VAULT_NAME="${VAULT_PATH##*/}"

# The allowlist: irreversible or outward-facing moves, plus writes to the
# identity vault itself. These are the moments where being wrong is expensive
# and where an arc has something to say. Deliberately NOT: reads, greps, tests,
# builds, status checks — the vast majority of calls, where a pointer block
# would be pure noise.
QUERY=""
case "$CMD" in
  *"git commit"*)              QUERY="committing work: what do I know about commit discipline, atomic commits, and verifying before claiming done" ;;
  *"git push"*|*"gh pr merge"*) QUERY="pushing or merging: publication is a one-way gate, review before merge, my authority to decide" ;;
  *"gh pr comment"*|*"gh pr review"*) QUERY="reviewing someone else's work: how I review, what I look for, holding a standard" ;;
esac

# A write aimed at the identity vault itself, whatever it is called. This used
# to fire on any command that merely NAMED the vault — every read, every
# `note get`, every index run — and never on Edit/Write, the tools arcs are
# actually written with (measured 2026-09-25: a reminder before every read, and
# none across three arcs written with Write). Now it fires on a write only: an
# Edit/Write/MultiEdit whose file is inside the vault, or a shell command that
# writes into it — a redirect whose target is in the vault, a file command
# (rm, mv, cp, tee, …), an in-place sed, a git add/rm/mv, or a vaultmind
# command that mutates notes.
IDENTITY_QUERY="writing to my identity vault: arc discipline, curation, never silently rewrite identity"
if [ -z "$QUERY" ] && [ -n "$FILE_PATH" ]; then
  case "$FILE_PATH" in
    "$VAULT_PATH"/*) QUERY="$IDENTITY_QUERY" ;;
  esac
fi
if [ -z "$QUERY" ] && [ -n "$CMD" ]; then
  WRITES=$(python3 -c "$(cat <<'PY'
import shlex, sys
cmd, name = sys.argv[1], sys.argv[2]
if name not in cmd:
    print(0); sys.exit()
try:
    lex = shlex.shlex(cmd, posix=True, punctuation_chars=True)
    lex.whitespace_split = True
    toks = list(lex)
except ValueError:
    print(0); sys.exit()
segs, cur = [], []
for t in toks:
    if t in (';', '|', '||', '&&', '&'):
        segs.append(cur); cur = []
    elif t not in ('(', ')', '{', '}'):
        cur.append(t)
segs.append(cur)
FILE_CMDS = {'rm', 'mv', 'cp', 'tee', 'touch', 'mkdir', 'rmdir', 'ln', 'truncate'}
GIT_WRITES = {'add', 'rm', 'mv', 'restore', 'checkout'}
WRAPPERS = {'sudo', 'env', 'command', 'time', 'nohup', 'xargs', 'exec'}
# vaultmind commands that change notes, as the leading subcommand words.
VM_MUTATIONS = {('note', 'create'), ('apply',), ('dataview', 'render'), ('doctor', 'heal'),
                ('frontmatter', 'set'), ('frontmatter', 'unset'), ('frontmatter', 'merge'),
                ('frontmatter', 'normalize'), ('frontmatter', 'fix')}
def strip_prefix(seg):
    """Drop NAME=value assignments and wrapper commands (with their flags)."""
    i = 0
    while i < len(seg):
        t = seg[i]
        if '=' in t and not t.startswith('-') and t.split('=', 1)[0].isidentifier():
            i += 1
        elif t.rsplit('/', 1)[-1] in WRAPPERS:
            i += 1
            while i < len(seg) and seg[i].startswith('-'):
                i += 1
        else:
            break
    return seg[i:]
def writes(seg, in_vault):
    if not in_vault and not any(name in t for t in seg):
        return False
    for i, t in enumerate(seg):
        # Inside the vault a relative target stays inside; an absolute or
        # home-relative one goes wherever it names.
        if t in ('>', '>>') and i + 1 < len(seg) and (
                name in seg[i + 1] or (in_vault and not seg[i + 1].startswith(('/', '~')))):
            return True
    seg = strip_prefix(seg)
    head = seg[0].rsplit('/', 1)[-1] if seg else ''
    args = seg[1:]
    if head in FILE_CMDS:
        return True
    if head == 'sed' and any(t.startswith('-i') or t == '--in-place' for t in args):
        return True
    if head == 'git':
        while len(args) >= 2 and args[0] == '-C':
            args = args[2:]
        return bool(args) and args[0] in GIT_WRITES
    if head == 'vaultmind':
        words = [t for t in args if not t.startswith('-')]
        if tuple(words[:1]) in VM_MUTATIONS or tuple(words[:2]) in VM_MUTATIONS:
            return True
        return any(t == '--fix' or t.startswith('--mark-reviewed') for t in args)
    return False
# A cd into the vault makes the following segments act inside it, where writes
# no longer name it; a cd anywhere else ends that.
in_vault, found = False, False
for s in segs:
    core = strip_prefix(s)
    if core and core[0] in ('cd', 'pushd'):
        in_vault = len(core) > 1 and name in core[1]
        continue
    if writes(s, in_vault):
        found = True
        break
print(1 if found else 0)
PY
)" "$CMD" "$IDENTITY_VAULT_NAME" 2>/dev/null || echo 0)
  [ "$WRITES" = "1" ] && QUERY="$IDENTITY_QUERY"
fi

[ -z "$QUERY" ] && exit 0

command -v vaultmind >/dev/null 2>&1 || exit 0
VAULTMIND=$(command -v vaultmind)
[ -d "$VAULT_PATH" ] || exit 0

LOG_DIR="${HOME}/.vaultmind/reach-hook"
mkdir -p "$LOG_DIR" 2>/dev/null

# Bound the query so a slow answer never delays the tool call this hook is
# standing in front of. `timeout` is GNU coreutils: every Linux box has it, NO
# stock macOS does — it arrives via Homebrew, as `timeout` or `gtimeout`.
#
# This script hardcoded `timeout 10`. On a stock Mac that command did not exist,
# the substitution came back empty, and the hook logged
# `"matched":true,"injected":false` — the same line it writes when the vault
# genuinely had nothing to say. Silent because a binary was missing, recorded as
# silent by choice. Resolve the binary; run unbounded when there is none.
# One definition for the bound, so the default and the message that reports it
# cannot drift. Raised from 10: measured 2026-09-09 on BGE-M3/ORT, a federated
# query costs 5.6s across two vaults and 10.5s across three, so the old bound
# guaranteed a kill the moment this hook federated.
HOOK_QUERY_TIMEOUT="${VAULTMIND_HOOK_QUERY_TIMEOUT:-25}"
TIMEOUT_CMD=""
if command -v timeout >/dev/null 2>&1; then
  TIMEOUT_CMD="timeout $HOOK_QUERY_TIMEOUT"
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_CMD="gtimeout $HOOK_QUERY_TIMEOUT"
fi

# Same relevance floor as vault-recall.sh: --quiet-on-no-match means an
# off-domain reach prints nothing rather than pointing at whatever ranked least
# badly. max-items 2 because this fires mid-task, where attention is scarcest.
# FEDERATION. An agent's memory is not one vault: mine is three, this hook
# searched one, and two findings from a single working week sat in the desk —
# retrievable there at z=+1.81 and z=+3.08 — unreachable from where the work
# happens. VAULTMIND_VAULTS (comma-separated paths) searches them together;
# results merge by cross-vault RRF, each vault judges relevance against its OWN
# noise floor, and the winning vault delivers the body.
#
# Unset ⇒ exactly today's single-vault call. Additive by construction: no
# existing adopter changes behaviour on upgrade.
VAULT_ARGS=(--vault "$VAULT_PATH")
if [ -n "${VAULTMIND_VAULTS:-}" ]; then
  VAULT_ARGS=(--vaults "$VAULTMIND_VAULTS")
fi

POINTERS=$(VAULTMIND_CALLER=vaultmind-reach-hook VAULTMIND_USER_SESSION_ID="$HOOK_SESSION_ID" $TIMEOUT_CMD "$VAULTMIND" ask "$QUERY" \
  "${VAULT_ARGS[@]}" \
  --max-items 2 \
  --budget 900 \
  --quiet-on-no-match \
  --excerpt 80 2>/dev/null)
ASK_STATUS=$?

TS=$(date +%Y%m%dT%H%M%S)
# A FAILED reach says so; an EMPTY one does not.
#
# This hook did not capture the exit status at all, so a killed query and a
# vault with nothing to say wrote the same log line — "matched":true,
# "injected":false — which is verbatim the failure class described at the top
# of this file. It was closed for the missing-`timeout`-binary case and left
# open for the timed-out one.
if [ "$ASK_STATUS" != "0" ]; then
  if [ "$ASK_STATUS" = "124" ]; then
    echo "VAULT — reach timed out after ${HOOK_QUERY_TIMEOUT}s; your notes were NOT consulted for this file."
  else
    echo "VAULT — reach failed (exit $ASK_STATUS); your notes were NOT consulted for this file."
  fi
  printf '{"timestamp":"%s","matched":true,"injected":false,"ask_status":%d}\n' "$TS" "$ASK_STATUS" \
    >> "$LOG_DIR/${TS}-reach.jsonl" 2>/dev/null
  exit 0
fi
if [ -z "$POINTERS" ]; then
  printf '{"timestamp":"%s","matched":true,"injected":false,"ask_status":0}\n' "$TS" \
    >> "$LOG_DIR/${TS}-reach.jsonl" 2>/dev/null
  exit 0
fi

# The footer must name the vault the note CAME FROM, not the primary one.
#
# Under federation the delivering vault is whichever one owned the top hit, and
# `note get` takes only --vault. Pointing at VAULT_PATH sent the reader to a
# vault the note is not in: measured live, the reach hook delivered a
# vaultmind-mine journal and told me to fetch it from vaultmind-identity, where
# `note get` answers "No note found". Dead-end advice introduced by the
# federation work itself.
#
# `ask` already names the owner by directory basename ("delivering from X"), so
# map that back through VAULTMIND_VAULTS. Unfederated, or if the line is absent
# or unmatched, this returns VAULT_PATH and the single-vault behaviour is
# byte-identical.
resolve_delivering_vault() {
  local pointers="$1" owner path
  [ -z "${VAULTMIND_VAULTS:-}" ] && { printf '%s' "$VAULT_PATH"; return; }
  owner=$(printf '%s' "$pointers" | sed -n 's/.*delivering from \([A-Za-z0-9._-]*\).*/\1/p' | head -1)
  [ -z "$owner" ] && { printf '%s' "$VAULT_PATH"; return; }
  while IFS= read -r path; do
    [ -z "$path" ] && continue
    if [ "$(basename "$path")" = "$owner" ]; then printf '%s' "$path"; return; fi
  done <<< "$(printf '%s' "$VAULTMIND_VAULTS" | tr ',' '\n')"
  printf '%s' "$VAULT_PATH"
}
DELIVERING_VAULT=$(resolve_delivering_vault "$POINTERS")

printf '{"timestamp":"%s","matched":true,"injected":true,"chars":%d}\n' "$TS" "${#POINTERS}" \
  >> "$LOG_DIR/${TS}-reach.jsonl" 2>/dev/null

# permissionDecision "allow" is explicit: this hook informs, it never gates.
python3 -c "
import json, sys
pointers = sys.stdin.read()
vault = sys.argv[1]
print(json.dumps({
    'hookSpecificOutput': {
        'hookEventName': 'PreToolUse',
        'permissionDecision': 'allow',
        'additionalContext': (
            'VAULT — you are at a decision point, not a greeting. '
            'These are ranked against what you are about to do:\n\n'
            + pointers
            + '\nThat is the note\'s own text, not a pointer to it. Notes '
              'written as arcs carry their rule in a Principle section and that '
              'is what you get; others give their opening lines, which may be '
              'context rather than a rule. Act on it if it bears on this call — '
              'full note: vaultmind note get <id> --vault ' + vault + '.'
        ),
    }
}))
" "$DELIVERING_VAULT" <<< "$POINTERS"
