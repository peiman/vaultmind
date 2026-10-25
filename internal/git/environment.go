package git

import "strings"

// gitLocationVars are what git exports to a hook to say which repository it is
// in. Inherited, they point every git command at that repository instead of
// the one named with -C — a hook's GIT_INDEX_FILE once made a test's
// `git add` write into the calling worktree's index.
var gitLocationVars = []string{
	"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_COMMON_DIR=", "GIT_PREFIX=",
	"GIT_OBJECT_DIRECTORY=", "GIT_ALTERNATE_OBJECT_DIRECTORIES=",
}

// WithoutLocation drops repository-location variables but preserves discovery
// controls such as GIT_CEILING_DIRECTORIES. Set to empty they are still set,
// and git refuses an empty path, so they are removed.
func WithoutLocation(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !hasAnyPrefix(kv, gitLocationVars) {
			out = append(out, kv)
		}
	}
	return out
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
