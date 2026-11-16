//go:build !windows

package signerservice

import (
	"os"
	"syscall"
)

// launcherDirOwnerUID returns the uid that owns the file described by fi.
// ok is false when the platform does not report an owner.
func launcherDirOwnerUID(fi os.FileInfo) (uid uint32, ok bool) {
	st, isStat := fi.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, false
	}
	return st.Uid, true
}

// currentUID returns the current user's uid as the type Stat_t reports.
func currentUID() uint32 {
	return uint32(os.Getuid()) //nolint:gosec // G115: uid is non-negative; uint32 conversion is intentional
}
