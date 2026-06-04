// Package shellparse provides shell-quoting-aware preprocessing for
// hooks that pattern-match Bash command strings.
//
// Why this exists: any hook that runs regex over a Bash command line
// — auto-RAG drift detection, command-allowlist gates, taint trackers
// — must skip content inside heredoc bodies, single-quoted strings,
// and double-quoted strings. The shell sees those regions as literal
// text; a naïve regex sees command separators (`|`, `;`, `&`) and
// command verbs inside them and fires false positives. This was a
// real failure mode discovered during the companion project's auto-RAG dogfood
// 2026-05-06/07: drift verbs in git commit-message heredoc bodies
// tripped the guard twice during commit authoring.
//
// Origin: ported from the companion project v0.3 stable
// (a companion project's shell-strip.sh,
// commits b0c7ee4 + 0ee2f89, 2026-05-07). The bash reference is the
// load-bearing implementation; this Go port is a translation, not a
// reinterpretation.
package shellparse

import "strings"

// state identifies which lexical region the walker is in. The four
// states correspond exactly to the bash reference's state machine
// (`shell-strip.sh` v0.3).
type state int

const (
	stateOutside state = iota
	stateInSingle
	stateInDouble
	stateInHeredoc
)

// StripShellQuoting returns the OUTSIDE skeleton of a Bash command
// string with heredoc bodies and quoted regions removed. Used by
// drift-detection regexes that must not match command separators or
// verbs that appear inside literal-quoted text.
//
// Coverage (intentional, v0.3):
//   - heredoc starts: <<MARKER, <<-MARKER, <<'MARKER', <<"MARKER",
//     <<-'MARKER', <<-"MARKER". The <<- variant strips leading tabs
//     (only tabs, not spaces, per shell semantics) when matching the
//     close marker.
//   - single-quoted strings ('...'): no escapes per shell semantics;
//     close on next '.
//   - double-quoted strings ("..."): \X is an escape (so \" doesn't
//     close); close on unescaped ".
//
// Coverage NOT included (deferred TODOs from the companion project v0.3 spec):
//   - here-strings (<<<): body is single token, low false-positive
//     risk.
//   - nested same-marker heredocs: exotic shell construct; not
//     observed in the companion project workflow.
//   - heredocs inside $(...) command substitution: this preprocessor
//     is line-oriented for heredoc bodies, so nested cases may produce
//     slightly off skeletons. Acceptable for drift detection (regex
//     is permissive about extra OUTSIDE content).
//
// Pure function: no I/O, no allocations beyond the output builder.
// Byte-oriented (matches bash semantics); content bytes pass through
// unchanged whether dropped or copied, so multi-byte UTF-8 in
// quoted/heredoc bodies is handled implicitly.
func StripShellQuoting(cmd string) string {
	var out strings.Builder
	out.Grow(len(cmd))

	st := stateOutside
	var marker string
	var allowTabs bool // <<- variant: leading tabs stripped on close-marker check

	i, n := 0, len(cmd)
	for i < n {
		// IN_HEREDOC is line-oriented: consume until a line equals
		// marker (with optional leading-tab tolerance for <<- variant).
		if st == stateInHeredoc {
			lineEnd := strings.IndexByte(cmd[i:], '\n')
			var line string
			var nextI int
			if lineEnd == -1 {
				line = cmd[i:]
				nextI = n
			} else {
				line = cmd[i : i+lineEnd]
				nextI = i + lineEnd + 1
			}
			check := line
			if allowTabs {
				check = strings.TrimLeft(check, "\t")
			}
			if check == marker {
				// Close marker — emit the marker line so OUTSIDE
				// structure past this point is preserved.
				out.WriteString(line)
				if lineEnd != -1 {
					out.WriteByte('\n')
				}
				st = stateOutside
				marker = ""
				allowTabs = false
			}
			// Body lines (including bodies of unclosed heredocs) are dropped.
			i = nextI
			continue
		}

		ch := cmd[i]

		if st == stateOutside {
			// Heredoc opener detection: <<-?\s*['"]?MARKER['"]?
			if i+1 < n && cmd[i] == '<' && cmd[i+1] == '<' {
				if newI, ok := tryHeredocOpener(cmd, i, &out, &marker, &allowTabs); ok {
					st = stateInHeredoc
					i = newI
					continue
				}
				// No marker captured — fall through and treat `<<` as
				// literal characters.
			}

			switch ch {
			case '\'':
				st = stateInSingle
				i++
				continue
			case '"':
				st = stateInDouble
				i++
				continue
			default:
				out.WriteByte(ch)
				i++
				continue
			}
		}

		if st == stateInSingle {
			// No escape sequences in single-quoted strings (shell semantics).
			if ch == '\'' {
				st = stateOutside
			}
			i++
			continue
		}

		if st == stateInDouble {
			// \X is an escape — \" does not close.
			if ch == '\\' && i+1 < n {
				i += 2
				continue
			}
			if ch == '"' {
				st = stateOutside
			}
			i++
			continue
		}

		// Unreachable today — the four states above exhaust the enum.
		// The explicit advance prevents an infinite loop if a 5th state
		// is added without its own handler block.
		i++
	}

	return out.String()
}

// tryHeredocOpener attempts to consume a heredoc opener starting at
// i (where cmd[i:i+2] == "<<"). On success, emits the opener line
// (up to its terminating newline) into out, sets *marker and
// *allowTabs, and returns the new index past the opener line plus
// true. On failure (no identifier marker found), returns 0, false
// and leaves out/marker/allowTabs untouched so the caller can treat
// `<<` as literal. Parsing is delegated to parseHeredocOpener so the
// line-oriented StripCommentsAndBlanks reuses the exact same opener
// semantics (SSOT).
func tryHeredocOpener(cmd string, i int, out *strings.Builder, marker *string, allowTabs *bool) (int, bool) {
	m, tabs, endOfLine, nextI, ok := parseHeredocOpener(cmd, i)
	if !ok {
		return 0, false
	}
	// Emit the opener line up to its terminating newline (or end).
	out.WriteString(cmd[i:endOfLine])
	if endOfLine < len(cmd) {
		out.WriteByte('\n')
	}
	*marker = m
	*allowTabs = tabs
	return nextI, true
}

// parseHeredocOpener parses a heredoc opener starting at i (where
// cmd[i:i+2] == "<<") WITHOUT emitting anything. It returns the marker
// identifier, whether the <<- (leading-tab-stripping) variant was
// used, the index of the opener line's terminating newline (or
// len(cmd) when the opener ends the input), the index of the first
// body byte (past that newline), and ok. On a malformed opener (no
// identifier marker) it returns ok=false. Pure — the caller decides
// what, if anything, to emit.
func parseHeredocOpener(cmd string, i int) (marker string, allowTabs bool, endOfLine, nextI int, ok bool) {
	n := len(cmd)
	j := i + 2 // past the leading `<<`

	if j < n && cmd[j] == '-' {
		allowTabs = true
		j++
	}
	for j < n && (cmd[j] == ' ' || cmd[j] == '\t') {
		j++
	}

	var quote byte
	if j < n && (cmd[j] == '"' || cmd[j] == '\'') {
		quote = cmd[j]
		j++
	}

	mStart := j
	for j < n && isIdentChar(cmd[j]) {
		j++
	}
	if j == mStart {
		// No identifier captured — not a valid heredoc opener.
		return "", false, 0, 0, false
	}
	marker = cmd[mStart:j]

	if quote != 0 && j < n && cmd[j] == quote {
		j++
	}

	nl := strings.IndexByte(cmd[j:], '\n')
	if nl == -1 {
		endOfLine = n
		nextI = n
	} else {
		endOfLine = j + nl
		nextI = endOfLine + 1
	}
	return marker, allowTabs, endOfLine, nextI, true
}

// isIdentChar reports whether b is a valid heredoc-marker character
// (alnum or underscore — matching bash heredoc identifier semantics).
func isIdentChar(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b == '_':
		return true
	default:
		return false
	}
}

// StripCommentsAndBlanks returns the behavioral skeleton of a shell
// SCRIPT: full-line comments and blank lines removed, while heredoc
// bodies and multi-line quoted-string contents are preserved verbatim.
// Two scripts with equal skeletons differ only in comments or blank
// lines — i.e. they behave identically.
//
// Hook-drift detection uses this so a comment-only divergence (e.g. an
// installed script that kept richer real-name annotations than the
// sanitized canonical) is NOT reported as drift, while a real CODE
// change still is. It is the script-level analogue of StripShellQuoting
// (which is command-string-level and drops the opposite regions:
// quoted/heredoc content, keeping comments).
//
// Scope (intentional): only FULL-LINE comments are stripped — a line
// whose first non-blank byte is `#`, excluding a line-1 `#!` shebang
// (which selects the interpreter and so is behavioral; a `#!` on any
// later line is an ordinary comment and is dropped).
// Trailing/inline comments (`cmd # note`) are KEPT: stripping them
// needs word-boundary analysis and would risk false negatives, whereas
// keeping them at worst reports a harmless trailing-comment-only drift.
// A `#`-leading line inside a heredoc body or a multi-line quoted
// string is literal content, not a comment, so it is preserved; quote
// and heredoc state is tracked across lines with the same lexical model
// as StripShellQuoting.
func StripCommentsAndBlanks(script string) string {
	var out strings.Builder
	out.Grow(len(script))

	st := stateOutside
	var marker string
	var allowTabs bool

	chunks := strings.SplitAfter(script, "\n")
	for idx, chunk := range chunks {
		// SplitAfter yields a trailing "" after a final newline; skip it
		// so a script ending in "\n" doesn't grow a phantom empty line.
		if idx == len(chunks)-1 && chunk == "" {
			break
		}
		body := strings.TrimSuffix(chunk, "\n")

		switch st {
		case stateInHeredoc:
			// Heredoc body is literal content — keep verbatim. Close on a
			// line equal to the marker (leading tabs tolerated for <<-).
			out.WriteString(chunk)
			check := body
			if allowTabs {
				check = strings.TrimLeft(check, "\t")
			}
			if check == marker {
				st, marker, allowTabs = stateOutside, "", false
			}
			continue
		case stateInSingle, stateInDouble:
			// Continuation of a multi-line quoted string — keep verbatim,
			// advancing state in case the quote closes on this line.
			out.WriteString(chunk)
			st, marker, allowTabs = advanceWithinLine(body, st, marker, allowTabs)
			continue
		}

		// stateOutside, at the start of a fresh line. Trim `\r` too so a
		// CRLF blank line is dropped like its LF twin.
		trimmed := strings.Trim(body, " \t\r")
		switch {
		case trimmed == "":
			// blank line — drop
		case trimmed[0] == '#' && (idx != 0 || !strings.HasPrefix(trimmed, "#!")):
			// full-line comment — drop. A `#!` on line 1 is the shebang
			// (kept); `#!` anywhere else is an ordinary comment.
		default:
			out.WriteString(chunk)
			st, marker, allowTabs = advanceWithinLine(body, stateOutside, "", false)
		}
	}

	return out.String()
}

// advanceWithinLine scans one line starting in state st (stateOutside,
// stateInSingle, or stateInDouble) and returns the lexical state at the
// line's end. It is line-oriented: a heredoc opener flips the state to
// stateInHeredoc (the body begins on the following line, which the
// caller handles), so any text after the opener on the same line does
// not affect the returned state. Heredoc-opener semantics are shared
// with StripShellQuoting via parseHeredocOpener.
func advanceWithinLine(line string, st state, marker string, allowTabs bool) (state, string, bool) {
	n := len(line)
	for i := 0; i < n; {
		switch st {
		case stateOutside:
			if i+1 < n && line[i] == '<' && line[i+1] == '<' {
				if m, tabs, _, _, ok := parseHeredocOpener(line, i); ok {
					return stateInHeredoc, m, tabs
				}
			}
			switch line[i] {
			case '\'':
				st = stateInSingle
			case '"':
				st = stateInDouble
			}
			i++
		case stateInSingle:
			// No escapes in single-quoted strings (shell semantics).
			if line[i] == '\'' {
				st = stateOutside
			}
			i++
		case stateInDouble:
			// \X is an escape — \" does not close.
			if line[i] == '\\' && i+1 < n {
				i += 2
				continue
			}
			if line[i] == '"' {
				st = stateOutside
			}
			i++
		default:
			i++
		}
	}
	return st, marker, allowTabs
}
