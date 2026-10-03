package pathglob_test

import (
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/pathglob"
	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	for _, c := range []struct {
		pattern, rel string
		want         bool
	}{
		{"cmd/tree.go", "cmd/tree.go", true},
		{"cmd/tree.go", "cmd/tree_test.go", false},
		{"cmd/*.go", "cmd/tree.go", true},
		{"cmd/*.go", "cmd/sub/tree.go", false},
		{"internal/navigate/**", "internal/navigate/navigate.go", true},
		{"internal/navigate/**", "internal/navigate/deep/x.go", true},
		{"internal/navigate/**", "internal/navigator/x.go", false},
		{"internal/navigate/", "internal/navigate/navigate.go", true},
		{"**/*.sh", "internal/hookscripts/vault-reach.sh", true},
		{"**/*.sh", "vault-reach.sh", true},
		{"internal/**/embed.go", "internal/hookscripts/embed.go", true},
		{"internal/**/embed.go", "internal/embed.go", true},
		{"./cmd/tree.go", "cmd/tree.go", true},
	} {
		assert.Equal(t, c.want, pathglob.Match(c.pattern, c.rel), "%s vs %s", c.pattern, c.rel)
	}
}

// A run of ** is one **; unfolded, each extra one multiplied the search and a
// pathological pattern took seconds per file.
func TestMatch_ARunOfAnyDepthStaysFast(t *testing.T) {
	pattern := strings.Repeat("**/", 20) + "nomatch.go"
	rel := strings.Repeat("d/", 20) + "file.go"
	start := time.Now()
	assert.False(t, pathglob.Match(pattern, rel))
	assert.Less(t, time.Since(start), 100*time.Millisecond)
	assert.True(t, pathglob.Match("**/**/file.go", "a/b/file.go"))
}
