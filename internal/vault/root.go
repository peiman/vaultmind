package vault

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveRoot returns the vault directory the operator named.
//
// filepath.WalkDir lstats its root and does not descend when that root is a
// symlink: the link is not a directory, so the walk yields one entry and
// stops. A vault addressed through a stable pointer (vaultmind-identity ->
// the real directory) then looks empty, and an indexer that treats missing
// files as deleted wipes the index.
//
// Only that named path is resolved, and only when its final element is a
// symlink. Links inside the vault stay links — SkipSymlink is the rule for
// those, and resolving them is the read/write hole it closes. A directory
// that is not itself a link is returned from Abs unchanged, so an ancestor
// such as /var -> /private/var does not rewrite every vault path. A path
// that does not exist yet has nothing to resolve; callers that confine
// before the directory exists get the absolute path.
//
// One function, used by the walk and by confinement, so both agree on the root.
func ResolveRoot(vaultRoot string) (string, error) {
	abs, err := filepath.Abs(vaultRoot)
	if err != nil {
		return "", fmt.Errorf("resolving vault root: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return abs, nil //nolint:nilerr // missing or not a link: nothing to resolve; Abs is the root
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil //nolint:nilerr // unresolvable link: keep the path the operator named
	}
	return resolved, nil
}
