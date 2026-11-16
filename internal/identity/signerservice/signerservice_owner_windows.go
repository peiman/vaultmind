//go:build windows

package signerservice

import "os"

// launcherDirOwnerUID reports no owner on Windows, which has no Unix uid; the
// launcher (launchd) feature is macOS-only, so the ownership check is skipped.
func launcherDirOwnerUID(_ os.FileInfo) (uid uint32, ok bool) {
	return 0, false
}

// currentUID is unused on Windows beyond satisfying the shared call site.
func currentUID() uint32 {
	return 0
}
