//go:build !darwin && !linux

package signer

import (
	"errors"
	"net"
)

// peerUID is unavailable on platforms without a Unix-domain peer-credential
// syscall (SO_PEERCRED on linux / LOCAL_PEERCRED on darwin). The custody
// signer's same-uid gate therefore FAILS CLOSED here: it rejects every
// connection rather than trusting an unauthenticated peer. The signer is a
// unix-only custody model (Contract-B Door 1); this stub exists only so the
// binary still cross-compiles for non-unix release artifacts (e.g. Windows) —
// the `identity signer` daemon is not supported on those platforms.
func peerUID(_ *net.UnixConn) (uint32, error) {
	return 0, errors.New("peer-credential check unsupported on this platform: the Contract-B custody signer requires linux or darwin")
}
