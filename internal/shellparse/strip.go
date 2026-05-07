// Package shellparse provides shell-quoting-aware preprocessing for
// hooks that pattern-match Bash command strings.
//
// Why this exists: any hook that runs regex over a Bash command line
// — auto-RAG drift detection, command-allowlist gates, taint trackers
// — must skip content inside heredoc bodies, single-quoted strings,
// and double-quoted strings. The shell sees those regions as literal
// text; a naïve regex sees command separators (`|`, `;`, `&`) and
// command verbs inside them and fires false positives. This was a
// real failure mode discovered during workhorse's auto-RAG dogfood
// 2026-05-06/07: drift verbs in git commit-message heredoc bodies
// tripped the guard twice during commit authoring.
//
// Origin: ported from workhorse v0.3 stable
// (`/Users/peiman/dev/workhorse/.claude/scripts/shell-strip.sh`,
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
// Coverage NOT included (deferred TODOs from workhorse v0.3 spec):
//   - here-strings (<<<): body is single token, low false-positive
//     risk.
//   - nested same-marker heredocs: exotic shell construct; not
//     observed in workhorse workflow.
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
		}
	}

	return out.String()
}

// tryHeredocOpener attempts to consume a heredoc opener starting at
// i (where cmd[i:i+2] == "<<"). On success, emits the opener line
// (up to its terminating newline) into out, sets *marker and
// *allowTabs, and returns the new index past the opener line plus
// true. On failure (no identifier marker found), returns 0, false
// and leaves out/marker/allowTabs untouched so the caller can treat
// `<<` as literal.
func tryHeredocOpener(cmd string, i int, out *strings.Builder, marker *string, allowTabs *bool) (int, bool) {
	n := len(cmd)
	j := i + 2 // past the leading `<<`

	tabsVariant := false
	if j < n && cmd[j] == '-' {
		tabsVariant = true
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
		return 0, false
	}
	newMarker := cmd[mStart:j]

	if quote != 0 && j < n && cmd[j] == quote {
		j++
	}

	// Emit the opener line up to its terminating newline (or end).
	nl := strings.IndexByte(cmd[j:], '\n')
	var endOfLine, nextI int
	if nl == -1 {
		endOfLine = n
		nextI = n
	} else {
		endOfLine = j + nl
		nextI = endOfLine + 1
	}
	out.WriteString(cmd[i:endOfLine])
	if nl != -1 {
		out.WriteByte('\n')
	}

	*marker = newMarker
	*allowTabs = tabsVariant
	return nextI, true
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
