package hooks

import (
	"regexp"
	"strings"
)

// groupsForScript preserves group boundaries, including every command for the
// script. Multiple commands in one group do not make it hand-wired.
func groupsForScript(groups []hookGroup, script string) []hookGroup {
	var out []hookGroup
	for _, group := range groups {
		matched := hookGroup{Matcher: group.Matcher}
		for _, h := range group.Hooks {
			if commandReferencesScript(h.Command, script) {
				matched.Hooks = append(matched.Hooks, h)
			}
		}
		if len(matched.Hooks) > 0 {
			out = append(out, matched)
		}
	}
	return out
}

func addDuplicateNotices(status *EventStatus, shared, local []hookGroup) {
	identical, different := false, false
	for _, a := range shared {
		for _, b := range local {
			if !matchersMayOverlap(a.Matcher, b.Matcher) {
				continue
			}
			for _, ah := range a.Hooks {
				for _, bh := range b.Hooks {
					if ah.Command == bh.Command {
						identical = true
					} else {
						different = true
					}
				}
			}
		}
	}
	if identical {
		status.Notes = []string{"wired in both settings files; Claude Code runs it once — keep one"}
	}
	if different {
		status.Warnings = []string{"Warning: hooks can fire twice; remove one file's wiring. Neither file is removed automatically."}
	}
}

// Claude Code treats plain names and lists as exact matches, and other patterns
// as unanchored regular expressions. See https://code.claude.com/docs/en/hooks#matcher-patterns.
var exactMatcherList = regexp.MustCompile(`^[A-Za-z0-9_ ,|\-]+$`)

func matchersMayOverlap(a, b string) bool {
	if a == "" || b == "" || a == "*" || b == "*" {
		return true
	}
	if !exactMatcherList.MatchString(a) {
		a, b = b, a
	}
	if !exactMatcherList.MatchString(a) {
		ap, bp := anchoredMatcherPrefix(a), anchoredMatcherPrefix(b)
		if ap != "" && bp != "" && !strings.HasPrefix(ap, bp) && !strings.HasPrefix(bp, ap) {
			return false
		}
		// Arbitrary regex intersection (including JavaScript-only syntax) is
		// not decided here. Preserve an advisory for potentially shared calls.
		return true
	}
	var matches func(string) bool
	if exactMatcherList.MatchString(b) {
		names := strings.FieldsFunc(b, func(r rune) bool { return r == '|' || r == ',' })
		matches = func(name string) bool {
			for _, other := range names {
				if name == strings.TrimSpace(other) {
					return true
				}
			}
			return false
		}
	} else {
		re, err := regexp.Compile(b)
		if err != nil {
			return true // JavaScript regex syntax may be valid but unsupported by Go.
		}
		matches = re.MatchString
	}
	for _, name := range strings.FieldsFunc(a, func(r rune) bool { return r == '|' || r == ',' }) {
		if matches(strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// Disjoint mandatory prefixes prove that two anchored regexes cannot match
// the same tool name. Conservatively reject alternatives: an initial anchor
// may apply to only one branch (for example, ^Read|ReadMore).
func anchoredMatcherPrefix(pattern string) string {
	if !strings.HasPrefix(pattern, "^") || strings.Contains(pattern, "|") {
		return ""
	}
	re, err := regexp.Compile(strings.TrimPrefix(pattern, "^"))
	if err != nil {
		return ""
	}
	prefix, _ := re.LiteralPrefix()
	return prefix
}
