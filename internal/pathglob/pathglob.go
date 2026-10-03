// Package pathglob is the glob language of `paths:` in note frontmatter:
// slash-separated segments matched as in path.Match, `**` for any number of
// segments, and a trailing `/` covering everything beneath. The code-map
// hook and the crawler's --include/--exclude share it, so one pattern means
// the same thing everywhere.
package pathglob

import (
	"path"
	"strings"
)

// anyDepth matches zero or more path segments.
const anyDepth = "**"

// Match reports whether rel (slash-separated, relative to a root) matches
// pattern. A leading "./" on the pattern is ignored.
func Match(pattern, rel string) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if strings.HasSuffix(pattern, "/") {
		pattern += anyDepth
	}
	return matchSegments(collapseAnyDepth(strings.Split(pattern, "/")), strings.Split(rel, "/"))
}

// collapseAnyDepth folds a run of `**` into one: they mean the same, and each
// extra one multiplied the search (a 13-long run took seconds per file).
func collapseAnyDepth(segs []string) []string {
	out := segs[:0:0]
	for _, s := range segs {
		if s == anyDepth && len(out) > 0 && out[len(out)-1] == anyDepth {
			continue
		}
		out = append(out, s)
	}
	return out
}

func matchSegments(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == anyDepth {
			for i := 0; i <= len(segs); i++ {
				if matchSegments(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}
