package signer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

// frameHeaderLen is the byte length of the big-endian uint32 length prefix that
// precedes every framed payload on the wire.
const frameHeaderLen = 4

// errFrameTooLarge is returned when a peer announces a payload larger than
// maxRequestBytes.
const errFrameTooLarge = "signer: frame exceeds maximum size"

// writeFrame writes a length-prefixed payload: a big-endian uint32 length
// followed by the bytes.
func writeFrame(conn net.Conn, payload []byte) error {
	if len(payload) > maxRequestBytes {
		return errors.New(errFrameTooLarge)
	}
	var hdr [frameHeaderLen]byte
	// len(payload) is bounded by maxRequestBytes (1 MiB) above, so the uint32
	// conversion cannot overflow.
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload))) //nolint:gosec // bounded by maxRequestBytes

	if _, err := conn.Write(hdr[:]); err != nil {
		return fmt.Errorf("signer: write frame header: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("signer: write frame payload: %w", err)
	}
	return nil
}

// readFrame reads a single length-prefixed payload, rejecting any frame larger
// than maxRequestBytes before allocating.
func readFrame(conn net.Conn) ([]byte, error) {
	var hdr [frameHeaderLen]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, fmt.Errorf("signer: read frame header: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxRequestBytes {
		return nil, errors.New(errFrameTooLarge)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("signer: read frame payload: %w", err)
	}
	return buf, nil
}

// writeOK writes a success response: the respOK status byte followed by a
// framed signature payload.
func writeOK(conn net.Conn, sig []byte) {
	_, _ = conn.Write([]byte{respOK})
	_ = writeFrame(conn, sig)
}

// writeErr writes a failure response: the respErr status byte followed by a
// framed error message. The signer NEVER includes key material in an error.
func writeErr(conn net.Conn, msg string) {
	_, _ = conn.Write([]byte{respErr})
	_ = writeFrame(conn, []byte(msg))
}
