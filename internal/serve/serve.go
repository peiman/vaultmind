// Package serve keeps one vaultmind process warm and answers the CLI's
// commands from it. A recall `ask` spent about 0.7 s of its 1.6 s loading
// the embedding model in every new process; a resident process loads it
// once. The server runs the real command for each request (see cmd's serve),
// so this package is only the transport: a private Unix socket, one request
// at a time, an idle timeout.
package serve

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Request is one CLI invocation: its arguments, working directory and
// environment.
type Request struct {
	Args []string          `json:"args"`
	Dir  string            `json:"dir"`
	Env  map[string]string `json:"env"`
}

// Response is what the command printed and the exit code it ended with.
type Response struct {
	Stdout []byte `json:"stdout"`
	Stderr []byte `json:"stderr"`
	Code   int    `json:"code"`
}

// Exec runs one request.
type Exec func(Request) Response

var (
	// ErrNoServer means nothing answers on the socket; the caller runs the
	// command itself.
	ErrNoServer = errors.New("no vaultmind server is running")
	// ErrAlreadyServing means another server already answers on the socket.
	ErrAlreadyServing = errors.New("a vaultmind server is already running on this socket")
	// ErrUnsafeDir means the socket's directory could be reached by another
	// user; neither side uses a socket there.
	ErrUnsafeDir = errors.New("unsafe socket directory")
)

// maxSocketPath stays under the Unix socket path limit (104 bytes on macOS).
const maxSocketPath = 100

// SocketPath is where the server for this version listens. The version is
// in the name, so two installed versions each get their own server and an
// upgrade never answers from old code. A state dir too deep for a socket
// path falls back to a short per-user name under the temp dir. Either way the
// socket's directory must be private to the user (see checkPrivateDir).
func SocketPath(stateDir, version string) string {
	name := "serve-" + strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, version) + ".sock"
	// Its own directory, created 0700: the state dir itself may be readable
	// by others (an XDG state dir is often 0755).
	if p := filepath.Join(stateDir, "serve", name); len(p) <= maxSocketPath {
		return p
	}
	sum := sha256.Sum256([]byte(stateDir + "\x00" + version))
	return filepath.Join(os.TempDir(), fmt.Sprintf("vaultmind-%d", os.Getuid()), fmt.Sprintf("s-%x.sock", sum[:6]))
}

// Serve answers requests on sock until ctx ends or no request arrives for
// idle, then removes the socket. Requests run one at a time.
func Serve(ctx context.Context, sock string, exec Exec, idle time.Duration) error {
	ln, err := listen(sock)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(sock) }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	activity := make(chan struct{}, 1)
	go func() {
		timer := time.NewTimer(idle)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
			case <-timer.C:
			case <-activity:
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(idle)
				continue
			}
			_ = ln.Close()
			return
		}
	}()

	var mu sync.Mutex
	var inFlight sync.WaitGroup
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Closed: idle, or ctx ended. Finish the requests already
			// accepted, so a signalled server still answers them.
			inFlight.Wait()
			return nil
		}
		touch(activity)
		inFlight.Add(1)
		go func() {
			defer inFlight.Done()
			defer func() { _ = conn.Close() }()
			var req Request
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				return
			}
			mu.Lock()
			resp := run(exec, req)
			mu.Unlock()
			touch(activity)
			_ = json.NewEncoder(conn).Encode(resp)
		}()
	}
}

// touch notes a request without blocking.
func touch(activity chan struct{}) {
	select {
	case activity <- struct{}{}:
	default:
	}
}

// run calls exec, turning a panic into a failed response so one bad request
// cannot take the server down.
func run(exec Exec, req Request) (resp Response) {
	defer func() {
		if r := recover(); r != nil {
			resp = Response{Stderr: []byte(fmt.Sprintf("vaultmind serve: %v\n", r)), Code: 70}
		}
	}()
	return exec(req)
}

// listen opens a private socket at sock. A live server already there is
// ErrAlreadyServing; a stale socket file from a crashed one is replaced.
func listen(sock string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		return nil, fmt.Errorf("creating the socket's directory: %w", err)
	}
	if err := checkPrivateDir(filepath.Dir(sock)); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		if alive(sock) {
			return nil, ErrAlreadyServing
		}
		_ = os.Remove(sock)
		if ln, err = net.Listen("unix", sock); err != nil {
			// Another server started at the same moment replaced the
			// stale socket first.
			if alive(sock) {
				return nil, ErrAlreadyServing
			}
			return nil, fmt.Errorf("listening on %s: %w", sock, err)
		}
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("making the socket private: %w", err)
	}
	return ln, nil
}

// alive reports whether a server answers on sock.
func alive(sock string) bool {
	c, err := net.DialTimeout("unix", sock, time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// Forward sends req to the server on sock and returns its response. With no
// server it returns ErrNoServer at once; a server slower than total is
// abandoned with an error.
func Forward(sock string, req Request, connect, total time.Duration) (Response, error) {
	if err := checkPrivateDir(filepath.Dir(sock)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Response{}, ErrNoServer
		}
		return Response{}, err
	}
	conn, err := net.DialTimeout("unix", sock, connect)
	if err != nil {
		// Nothing there, nothing listening, or a leftover non-socket file:
		// in each case no server will answer.
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTSOCK) {
			return Response{}, ErrNoServer
		}
		return Response{}, fmt.Errorf("connecting to the vaultmind server: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(total))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("sending to the vaultmind server: %w", err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("reading from the vaultmind server: %w", err)
	}
	return resp, nil
}
