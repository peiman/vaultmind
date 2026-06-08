//go:build darwin

package signer

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peerUID derives the UID of the process on the other end of a Unix-domain
// connection on darwin via getsockopt(SOL_LOCAL, LOCAL_PEERCRED), which returns
// an xucred whose Uid is the peer's effective UID. This is the same technique
// the Contract-B custody probe validated.
//
// TODO(contract-b hardening): under a dedicated service uid + launchd sandbox,
// this UID check becomes a real cross-trust-domain boundary; today the signer
// and CLI share a uid, so this gates "which uid may sign" but is not yet a
// privilege boundary.
func peerUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("syscall conn: %w", err)
	}
	var (
		cred    *unix.Xucred
		credErr error
	)
	ctrlErr := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if ctrlErr != nil {
		return 0, fmt.Errorf("control fd: %w", ctrlErr)
	}
	if credErr != nil {
		return 0, fmt.Errorf("getsockopt LOCAL_PEERCRED: %w", credErr)
	}
	return cred.Uid, nil
}
