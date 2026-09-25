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

# The directory the command runs in, which relative paths in it resolve
# against. The harness sends it; absent, the project directory stands in.
CMD_CWD=$(printf '%s' "$HOOK_INPUT" | python3 -c \
  "import json,sys
try:
    print(json.load(sys.stdin).get('cwd', '') or '')
except Exception:
    print('')" 2>/dev/null || echo "")

[ -z "$CMD" ] && [ -z "$FILE_PATH" ] && exit 0

VAULT_PATH="${VAULTMIND_VAULT:-${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$PWD}}/vaultmind-identity}"
# A trailing slash would leave the vault's name below empty, and an empty name
# matches every command.
VAULT_PATH="${VAULT_PATH%/}"
# A relative vault path (focalc sets ./vaultmind-vault) is relative to the
# project. Resolve it, or the absolute file_path of an Edit/Write never matches.
case "$VAULT_PATH" in
  /*) ;;
  *) VAULT_PATH="${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$PWD}}/${VAULT_PATH#./}" ;;
esac
# Every vault the project gave this hook: the primary, then VAULTMIND_VAULTS.
# A project can hold an identity vault AND knowledge vaults (mine: identity +
# desk; an adopter's: persona + project knowledge). A write to any of them is a
# moment worth reaching into — not only a write to the primary.
resolve_vault() {
  local v="${1%/}"
  case "$v" in
    /*) printf '%s' "$v" ;;
    *) printf '%s' "${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$PWD}}/${v#./}" ;;
  esac
}
VAULT_LIST=("$VAULT_PATH")
if [ -n "${VAULTMIND_VAULTS:-}" ]; then
  while IFS= read -r v; do
    [ -z "$v" ] && continue
    v=$(resolve_vault "$v")
    [ "$v" = "$VAULT_PATH" ] || VAULT_LIST+=("$v")
  done <<< "$(printf '%s' "$VAULTMIND_VAULTS" | tr ',' '\n')"
fi

# The allowlist: irreversible or outward-facing moves, plus writes to the
# identity vault itself. These are the moments where being wrong is expensive
# and where an arc has something to say. Deliberately NOT: reads, greps, tests,
# builds, status checks — the vast majority of calls, where a pointer block
# would be pure noise.
QUERY=""

# A commit is asked about by its own subject line. A fixed sentence returned
# the same notes before every commit, and a note seen eight times a day stops
# being read (measured 2026-09-25). Only a real `git commit` counts — the words
# inside a grep or an echo used to fire too. Prints nothing for no commit, "-"
# for a commit whose message is not on the command line, else the subject
# without its conventional-commit prefix.
COMMIT_SUBJECT=""
case "$CMD" in
  *git*commit*)
    COMMIT_SUBJECT=$(python3 -c "$(cat <<'PY'
import re, shlex, sys
try:
    lex = shlex.shlex(sys.argv[1], posix=True, punctuation_chars=True)
    lex.whitespace_split = True
    toks = list(lex)
except ValueError:
    sys.exit(0)
segments, cur = [], []
for t in toks:
    if t and set(t) <= set(";&|()"):
        segments.append(cur)
        cur = []
    else:
        cur.append(t)
segments.append(cur)
for argv in segments:
    while argv and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", argv[0]):
        argv = argv[1:]
    if not argv or argv[0] != "git":
        continue
    rest = argv[1:]
    while len(rest) > 1 and rest[0] in ("-C", "-c"):
        rest = rest[2:]
    if not rest or rest[0] != "commit":
        continue
    args, msg = rest[1:], None
    for i, a in enumerate(args):
        if a.startswith("--message="):
            msg = a.split("=", 1)[1]
        elif a == "--message" or (a.startswith("-") and not a.startswith("--") and a.endswith("m")):
            msg = args[i + 1] if i + 1 < len(args) else None
        if msg is not None:
            break
    lines = [l.strip() for l in (msg or "").splitlines() if l.strip()]
    if lines and lines[0].startswith("$(cat <<"):
        lines = lines[1:]
    subject = re.sub(r"^[a-z]+(\([^)]*\))?!?:\s*", "", lines[0]) if lines else ""
    print(subject or "-")
    sys.exit(0)
PY
)" "$CMD" 2>/dev/null)
    ;;
esac
case "$COMMIT_SUBJECT" in
  "")  ;;
  "-") QUERY="committing work: what do I know about commit discipline, atomic commits, and verifying before claiming done" ;;
  *)   QUERY="committing: $COMMIT_SUBJECT — what do I already know about this" ;;
esac

if [ -z "$QUERY" ]; then
  case "$CMD" in
    *"git push"*|*"gh pr merge"*) QUERY="pushing or merging: publication is a one-way gate, review before merge, my authority to decide" ;;
    *"gh pr comment"*|*"gh pr review"*) QUERY="reviewing someone else's work: how I review, what I look for, holding a standard" ;;
  esac
fi

# A write aimed at the identity vault itself, whatever it is called. This used
# to fire on any command that merely NAMED the vault — every read, every
# `note get`, every index run — and never on Edit/Write, the tools arcs are
# actually written with (measured 2026-09-25: a reminder before every read, and
# none across three arcs written with Write). Now it fires on a write only: an
# Edit/Write/MultiEdit whose file is inside the vault, or a shell command that
# writes into it — a redirect whose target is in the vault, a file command
# (rm, mv, cp, tee, …), an in-place sed, a git add/rm/mv, or a vaultmind
# command that mutates notes.
#
# What to ask depends on the vault being written, and it is asked of THAT vault
# alone. An identity vault (it has arcs/, which `vaultmind init` creates) gets
# arc discipline. A knowledge vault — a project's, or a desk — is not an
# identity: before writing a note there, what helps is what the vault already
# says about that topic (taken from the file name) and its conventions.
write_query_for() {
  local vault="$1" topic=""
  if [ -d "$vault/arcs" ]; then
    printf '%s' "writing to my identity vault: arc discipline, curation, never silently rewrite identity"
    return
  fi
  # Journal and desk files are named date-first; the date is not the topic.
  [ -n "$FILE_PATH" ] && topic=$(basename "$FILE_PATH" .md | sed -E 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}-//' | tr '_-' '  ')
  if [ -n "$topic" ]; then
    printf '%s' "writing about $topic: what this knowledge base already says about it, and its conventions"
  else
    printf '%s' "changing this project's knowledge base: its conventions, frontmatter, and what is already written"
  fi
}
TARGET_VAULT=""
if [ -z "$QUERY" ] && [ -n "$FILE_PATH" ]; then
  # The innermost vault wins when one is nested in another.
  for v in "${VAULT_LIST[@]}"; do
    case "$FILE_PATH" in
      "$v"/*) [ ${#v} -gt ${#TARGET_VAULT} ] && TARGET_VAULT="$v" ;;
    esac
  done
fi
if [ -z "$QUERY" ] && [ -z "$TARGET_VAULT" ] && [ -n "$CMD" ]; then
  WRITES=$(python3 -c "$(cat <<'PY'
import os, shlex, sys
# Prints the index (into argv[3:], one absolute vault path each) of the vault
# the command writes into, or -1. argv[2] is the directory the command runs in.
#
# Vaults are matched by PATH, not by a name found somewhere in the command: a
# name match took "kb-archive/n.md" for vault "kb", and could not tell two
# vaults that share a directory name apart. Each word is resolved against the
# directory it would be resolved against (moved by any cd), and the owning
# vault is the longest vault path that contains it.
cmd, start, vaults = sys.argv[1], sys.argv[2], sys.argv[3:]
def resolve(tok, cwd):
    tok = os.path.expanduser(tok)
    return os.path.normpath(tok if tok.startswith('/') else os.path.join(cwd, tok))
def owner(path):
    best, width = None, -1
    for i, v in enumerate(vaults):
        if v and (path == v or path.startswith(v + '/')) and len(v) > width:
            best, width = i, len(v)
    return best
# Cheap exit for the common case: a command that neither names a vault nor
# runs inside one (and does not cd anywhere) cannot write into one.
if (owner(os.path.normpath(start)) is None and 'cd' not in cmd
        and not any(os.path.basename(v) in cmd for v in vaults if v)):
    print(-1); sys.exit()
def hit(tok, cwd):
    if tok.startswith('-'):
        if '=' not in tok:
            return None
        tok = tok.split('=', 1)[1]
    return owner(resolve(tok, cwd))
try:
    lex = shlex.shlex(cmd, posix=True, punctuation_chars=True)
    lex.whitespace_split = True
    toks = list(lex)
except ValueError:
    print(-1); sys.exit()
segs, cur = [], []
for t in toks:
    if t in (';', '|', '||', '&&', '&'):
        segs.append(cur); cur = []
    elif t not in ('(', ')', '{', '}'):
        cur.append(t)
segs.append(cur)
FILE_CMDS = {'rm', 'mv', 'cp', 'tee', 'touch', 'mkdir', 'rmdir', 'ln', 'truncate'}
# These leave their sources untouched and write only their last argument, so
# that alone decides. mv is not here (moving out of a vault changes it), nor
# tee (it writes to every file it is given).
COPY_CMDS = {'cp', 'ln'}
# Redirect operators: they and the word after them are not arguments.
REDIRECTS = {'>', '>>', '<', '<<', '<<<', '>&', '<&', '&>', '&>>', '>|'}
def without_redirects(args):
    out, skip = [], False
    for i, t in enumerate(args):
        if skip:
            skip = False
            continue
        if t in REDIRECTS:
            skip = True
            continue
        if t.isdigit() and i + 1 < len(args) and args[i + 1] in REDIRECTS:
            continue
        out.append(t)
    return out
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
def writes(seg, cwd):
    """The vault index this segment writes into, or None."""
    for i, t in enumerate(seg):
        if t in ('>', '>>') and i + 1 < len(seg):
            h = hit(seg[i + 1], cwd)
            if h is not None:
                return h
    seg = strip_prefix(seg)
    head = seg[0].rsplit('/', 1)[-1] if seg else ''
    args = without_redirects(seg[1:])
    git_args = args
    if head == 'git':
        # git -C <dir> runs in <dir>: its paths resolve there.
        while len(git_args) >= 2 and git_args[0] == '-C':
            cwd = resolve(git_args[1], cwd)
            git_args = git_args[2:]
    hits = [h for h in (hit(t, cwd) for t in (git_args if head == 'git' else args)) if h is not None]
    if head == 'git' and not hits and git_args[1:2]:
        hits = [h for h in [owner(cwd)] if h is not None]
    if not hits:
        return None
    if head in COPY_CMDS:
        # Copying out of a vault writes somewhere else: the destination decides.
        dest = [t for t in args if not t.startswith('-')]
        return hit(dest[-1], cwd) if dest else None
    if head in FILE_CMDS:
        return hits[-1]
    if head == 'sed' and any(t.startswith('-i') or t == '--in-place' for t in args):
        return hits[-1]
    if head == 'git':
        return hits[0] if git_args and git_args[0] in GIT_WRITES else None
    if head == 'vaultmind':
        words = [t for t in args if not t.startswith('-')]
        if tuple(words[:1]) in VM_MUTATIONS or tuple(words[:2]) in VM_MUTATIONS:
            return hits[0]
        if any(t == '--fix' or t.startswith('--mark-reviewed') for t in args):
            return hits[0]
    return None
# A cd moves where relative paths in the segments after it resolve.
# (No apostrophes in this heredoc: bash 3.2 scans it for quotes.)
cwd, found = start, -1
for s in segs:
    core = strip_prefix(s)
    if core and core[0] in ('cd', 'pushd'):
        cwd = resolve(core[1], cwd) if len(core) > 1 else os.path.expanduser('~')
        continue
    w = writes(s, cwd)
    if w is not None:
        found = w
        break
print(found)
PY
)" "$CMD" "${CMD_CWD:-${VAULTMIND_PROJECT_DIR:-${CLAUDE_PROJECT_DIR:-$PWD}}}" "${VAULT_LIST[@]}" 2>/dev/null || echo -1)
  case "$WRITES" in
    ''|-1|*[!0-9]*) ;;
    *) TARGET_VAULT="${VAULT_LIST[$WRITES]:-}" ;;
  esac
fi
[ -z "$QUERY" ] && [ -n "$TARGET_VAULT" ] && QUERY=$(write_query_for "$TARGET_VAULT")

[ -z "$QUERY" ] && exit 0

command -v vaultmind >/dev/null 2>&1 || exit 0
VAULTMIND=$(command -v vaultmind)
[ -d "${TARGET_VAULT:-$VAULT_PATH}" ] || exit 0

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
if [ -n "$TARGET_VAULT" ]; then
  # A write is about one vault: ask that vault, not all of them.
  VAULT_ARGS=(--vault "$TARGET_VAULT")
elif [ -n "${VAULTMIND_VAULTS:-}" ]; then
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
DELIVERING_VAULT="$TARGET_VAULT"
[ -z "$DELIVERING_VAULT" ] && DELIVERING_VAULT=$(resolve_delivering_vault "$POINTERS")

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
