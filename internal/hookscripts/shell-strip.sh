#!/bin/bash
# shell-strip.sh — strip shell-quoted regions (heredocs, single-quoted
# strings, double-quoted strings) from a Bash command string. Returns
# the OUTSIDE skeleton — the command structure with all literal-quoted
# content removed.
#
# Purpose: drift-detection regexes (auto-rag-guard.sh) must not match
# pipe characters or command verbs that appear *inside* quoted regions
# or heredoc bodies. The shell sees those as literal text; the regex
# would otherwise see them as command separators and fire false
# positives. This script is the shell-quoting-awareness preprocessor
# that runs before any drift regex applies.
#
# Contract:
#   - reads command string from stdin
#   - writes OUTSIDE skeleton to stdout
#   - exit 0 always; pure function with no side effects
#
# Coverage (intentional, in v0.3):
#   - heredoc starts: <<MARKER, <<-MARKER, <<'MARKER', <<"MARKER",
#     <<-'MARKER', <<-"MARKER"
#   - single-quoted strings: '...'
#   - double-quoted strings: "..." (with \" backslash-escape support)
#
# Not covered (TODO at port time):
#   - here-strings (<<<) — body is single token, low false-positive risk
#   - nested same-marker heredocs — exotic; not observed in workhorse
#     workflow
#   - heredocs inside $(...) command substitution — preprocessor is
#     line-based for heredocs; nested cases may produce slightly off
#     skeletons. Acceptable for drift detection (regex is permissive
#     about extra OUTSIDE content).
#   - escape sequences in single-quoted strings — single quotes never
#     escape in shell semantics; this is correct.
#
# This is the bash-side reference implementation. Long-term home is
# vaultmind's `internal/shellparse/` Go package per the 2026-05-07
# cross-agent handoff (vaultmind-replies-workhorse-auto-rag-handoff.md).
# The structure of this script — single pure function, iterative state
# machine, no bash-isms — is designed to port one-to-one to Go.

set -uo pipefail

# Capture the Python source via heredoc to keep the inner quoting verbatim
# (single-quoted heredoc terminator prevents shell-side interpolation).
PYTHON_SRC=$(cat <<'PYEOF'
import sys

cmd = sys.stdin.read()
# Bash here-strings (<<<) append a trailing newline; strip it so the
# OUTSIDE skeleton matches the input byte-for-byte outside quoted regions.
if cmd.endswith('\n'):
    cmd = cmd[:-1]

out = []
state = 'OUTSIDE'   # OUTSIDE | IN_SINGLE | IN_DOUBLE | IN_HEREDOC
marker = None       # heredoc end-marker (when in IN_HEREDOC)
allow_tabs = False  # <<- variant: leading tabs are stripped on close-marker check

i = 0
n = len(cmd)

while i < n:
    # IN_HEREDOC: line-oriented; consume until a line equals marker
    # (with optional leading-tab tolerance for <<- variant).
    if state == 'IN_HEREDOC':
        nl = cmd.find('\n', i)
        line_end = nl if nl != -1 else n
        line = cmd[i:line_end]
        check = line.lstrip('\t') if allow_tabs else line
        if check == marker:
            # End of heredoc — emit the close-marker line so OUTSIDE
            # structure past this point is preserved.
            out.append(line)
            if nl != -1:
                out.append('\n')
            state = 'OUTSIDE'
            marker = None
            allow_tabs = False
        # Body lines (including the body of an unclosed heredoc) are dropped.
        i = (line_end + 1) if nl != -1 else n
        continue

    ch = cmd[i]

    if state == 'OUTSIDE':
        # Heredoc opener detection: <<-?\s*['"]?MARKER['"]?
        if cmd[i:i+2] == '<<':
            j = i + 2
            tabs_variant = False
            if j < n and cmd[j] == '-':
                tabs_variant = True
                j += 1
            # Skip optional whitespace between << and marker.
            while j < n and cmd[j] in ' \t':
                j += 1
            # Optional surrounding quote on the marker.
            quote = None
            if j < n and cmd[j] in "\"'":
                quote = cmd[j]
                j += 1
            # Marker: identifier characters.
            m_start = j
            while j < n and (cmd[j].isalnum() or cmd[j] == '_'):
                j += 1
            if j > m_start:
                new_marker = cmd[m_start:j]
                # Consume closing matching-quote if present.
                if quote and j < n and cmd[j] == quote:
                    j += 1
                # Emit the opener line up to its newline (or end), then switch
                # to IN_HEREDOC for the body.
                nl = cmd.find('\n', j)
                end_of_line = nl if nl != -1 else n
                out.append(cmd[i:end_of_line])
                if nl != -1:
                    out.append('\n')
                state = 'IN_HEREDOC'
                marker = new_marker
                allow_tabs = tabs_variant
                i = (end_of_line + 1) if nl != -1 else n
                continue
            # No marker captured — treat `<<` as literal characters; fall through.

        if ch == "'":
            state = 'IN_SINGLE'
            i += 1
            continue
        if ch == '"':
            state = 'IN_DOUBLE'
            i += 1
            continue
        # Default OUTSIDE character — emit.
        out.append(ch)
        i += 1
        continue

    if state == 'IN_SINGLE':
        # Single quotes: no escape sequences. Closes on the next '.
        if ch == "'":
            state = 'OUTSIDE'
        # Body characters are dropped.
        i += 1
        continue

    if state == 'IN_DOUBLE':
        # Double quotes: \X is an escape; \" does not close. Closes on unescaped ".
        if ch == '\\' and i + 1 < n:
            i += 2
            continue
        if ch == '"':
            state = 'OUTSIDE'
        # Body characters are dropped.
        i += 1
        continue

sys.stdout.write(''.join(out))
PYEOF
)

python3 -c "$PYTHON_SRC"
