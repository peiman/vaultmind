//go:build !windows

package serve

import (
	"fmt"
	"os"
	"syscall"
)

// checkPrivateDir refuses a socket directory another user could reach: one
// that is a symlink, not ours, or open to group or others. A request carries
// the caller's whole environment, so a socket another user planted (say in a
// /tmp directory they created first) must never receive one.
func checkPrivateDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(st.Uid) != os.Getuid() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: %s must be a directory owned by you with mode 0700", ErrUnsafeDir, dir)
	}
	return nil
}
