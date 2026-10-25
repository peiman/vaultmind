package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchersMayOverlap(t *testing.T) {
	for _, tc := range []struct {
		name, a, b string
		overlap    bool
	}{
		{"exact names", "Bash", "Edit", false},
		{"lists", "Bash|Edit", "Read, Edit", true},
		{"empty", "", "Edit", true},
		{"wildcard", "*", "Edit", true},
		{"regex and name", "^Ed.*", "Edit", true},
		{"name and regex", "Read", "^Ed.*", false},
		{"disjoint regex prefixes", "^mcp__one__.*", "^mcp__two__.*", false},
		{"overlapping regex prefixes", "^mcp__one__.*", "^mcp__one__read.*", true},
		{"disjoint anchored names", "^Read$", "^Edit$", false},
		{"unanchored alternative", "^Read|ReadMore", "^Edit.*", true},
		{"unanchored regexes", "Ed.*", "Read.*", true},
		{"unsupported JavaScript regex", "Edit", "(?=Edit)", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.overlap, matchersMayOverlap(tc.a, tc.b))
			assert.Equal(t, tc.overlap, matchersMayOverlap(tc.b, tc.a), "overlap is symmetric")
		})
	}
}
