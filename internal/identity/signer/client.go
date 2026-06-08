package signer

import (
	"errors"
	"fmt"
	"io"
	"net"
)

// errSignerUnreachable prefixes a dial failure so the CLI can FAIL CLOSED with
// a clear message instead of a silent nil signature.
const errSignerUnreachable = "signer: unreachable (is the signer running?)"

// Client dials the signer's 0600 Unix-domain socket and requests signatures.
// It holds NO key material — it exists so the CLI never opens the key file.
type Client struct {
	// SocketPath is the signer's 0600 Unix-domain socket.
	SocketPath string
}

// Sign sends the already-canonical bytes to the signer and returns the ed25519
// signature. It FAILS CLOSED (returns a non-nil error, never a nil signature)
// when the signer is unreachable or refuses the request. The caller MUST pass
// JCS-canonical bytes; the client does not canonicalize.
func (c *Client) Sign(canonicalBytes []byte) ([]byte, error) {
	if c.SocketPath == "" {
		return nil, errors.New("signer: client SocketPath is empty")
	}
	conn, err := net.Dial(network, c.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errSignerUnreachable, err)
	}
	defer func() { _ = conn.Close() }()

	if err := writeFrame(conn, canonicalBytes); err != nil {
		return nil, fmt.Errorf("signer: send request: %w", err)
	}

	status := make([]byte, 1)
	if _, err := io.ReadFull(conn, status); err != nil {
		return nil, fmt.Errorf("signer: read response status: %w", err)
	}
	payload, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("signer: read response: %w", err)
	}
	if status[0] != respOK {
		// Surface the signer's error message; fail closed.
		return nil, fmt.Errorf("signer refused: %s", string(payload))
	}
	return payload, nil
}
