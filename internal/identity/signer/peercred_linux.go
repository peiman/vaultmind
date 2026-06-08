//go:build linux

package signer

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peerUID derives the UID of the process on the other end of a Unix-domain
// connection on linux via getsockopt(SOL_SOCKET, SO_PEERCRED). The primary
// custody target is darwin (LOCAL_PEERCRED + xucred); this linux variant keeps
// the package buildable and testable on linux CI with the equivalent ucred
// mechanism.
func peerUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("syscall conn: %w", err)
	}
	var (
		ucred   *unix.Ucred
		credErr error
	)
	ctrlErr := raw.Control(func(fd uintptr) {
		ucred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if ctrlErr != nil {
		return 0, fmt.Errorf("control fd: %w", ctrlErr)
	}
	if credErr != nil {
		return 0, fmt.Errorf("getsockopt SO_PEERCRED: %w", credErr)
	}
	return ucred.Uid, nil
}
