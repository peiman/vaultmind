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
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// Request is one CLI invocation: its arguments, working directory and
// environment.
type Request struct {
	Args []string          `json:"args"`
	Dir  string            `json:"dir"`
	Env  map[string]string `json:"env"`
	// Received is when the server read the request, so the time it waited
	// for its turn counts in how long it took. Set by Serve, never sent.
	Received time.Time `json:"-"`
}

// Response is what the command printed and the exit code it ended with.
type Response struct {
	Stdout []byte `json:"stdout"`
	Stderr []byte `json:"stderr"`
	Code   int    `json:"code"`
}

// Exec runs one request.
type Exec func(Request) Response

// Shutdown releases what the resident process holds. Serve calls it once,
// after the accept loop ends: the idle timeout, or ctx ending.
type Shutdown func()

// Option configures Serve.
type Option func(*serveOptions)

type serveOptions struct {
	onShutdown Shutdown
	memCap     int64
	rss        func() int64
}

// WithMemoryCap makes the server exit after a request once rss() reports more
// than limit bytes. The request in hand is answered first; the next ask
// starts a fresh process. It bounds a resident process whatever makes it grow.
// A limit of zero or less, or a nil rss, turns the check off.
func WithMemoryCap(limit int64, rss func() int64) Option {
	return func(o *serveOptions) {
		o.memCap, o.rss = limit, rss
	}
}

func (o serveOptions) overMemoryCap() bool {
	return o.memCap > 0 && o.rss != nil && o.rss() > o.memCap
}

// WithShutdown sets the function Serve calls when it stops after it has
// taken the socket. A process that never takes the socket — another server
// already answers, or the directory is unsafe — does not call it.
func WithShutdown(fn Shutdown) Option {
	return func(o *serveOptions) {
		o.onShutdown = fn
	}
}

var (
	// ErrNoServer means nothing answers on the socket; the caller runs the
	// command itself.
	ErrNoServer = errors.New("no vaultmind server is running")
	// ErrUnanswered means the server never started the request: it could not
	// be reached, it closed the connection without a word, or the request
	// was still queued when the caller's queue wait ran out. The caller may
	// run the command itself. Any other Forward error means the server may
	// still be running the request, so running it again would run it twice.
	ErrUnanswered = errors.New("the vaultmind server did not take the request")
	// ErrAlreadyServing means another server already answers on the socket.
	ErrAlreadyServing = errors.New("a vaultmind server is already running on this socket")
	// ErrUnsafeDir means the socket's directory could be reached by another
	// user; neither side uses a socket there.
	ErrUnsafeDir = errors.New("unsafe socket directory")
)

// maxSocketPath stays under the Unix socket path limit (104 bytes on macOS).
const maxSocketPath = 100

// ackStarted is the one byte the server writes when a request starts
// running: after it holds the lock and the client is still waiting, and
// before the request runs. 0x06 (ACK) is not a byte JSON starts with, so an
// answer that skipped this byte cannot pass for a start.
//
// A client only ever talks to a server of its own build. SocketPath keys the
// socket by the state directory and the binary version, and the CLI passes a
// version that also carries the executable's identity, so a server built
// without this byte is never the process on the other end.
const ackStarted byte = 0x06

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

// Serve answers requests on sock until ctx ends or idle passes without a
// request running, then removes the socket. Accepting a connection does not
// reset that wait: a request queued behind one that never finishes must not
// keep the server alive (#421). Requests run one at a time. One whose client
// has gone before its turn is skipped, and one that starts is announced with
// ackStarted before it runs, so the client can tell "still queued" from
// "started". WithShutdown runs once the accept loop ends, and not when this
// process never takes the socket.
func Serve(ctx context.Context, sock string, exec Exec, idle time.Duration, opts ...Option) error {
	var opt serveOptions
	for _, o := range opts {
		o(&opt)
	}
	ln, err := listen(sock)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(sock) }()
	defer func() {
		if opt.onShutdown != nil {
			opt.onShutdown()
		}
	}()

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
		inFlight.Add(1)
		go func() {
			defer inFlight.Done()
			defer func() { _ = conn.Close() }()
			var req Request
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				return
			}
			req.Received = time.Now()
			mu.Lock()
			if clientGone(conn) {
				// Its client gave up while it waited its turn (a hook's
				// timeout killed it, or the queue cap ran out): no one would
				// read the answer, and running it would only delay the
				// requests behind it.
				mu.Unlock()
				log.Debug().Msg("vaultmind serve: skipped a queued request whose client had gone")
				return
			}
			// The client's queue deadline can expire at the same moment
			// clientGone returns false. This write may then succeed into the
			// socket buffer while the client has closed, and the ask runs
			// twice. That window is the time between clientGone and this
			// write; nothing else runs there.
			if n, err := conn.Write([]byte{ackStarted}); err != nil || n != 1 {
				mu.Unlock()
				log.Debug().Err(err).Msg("vaultmind serve: skipped a request whose client left before it started")
				return
			}
			touch(activity) // the request is running; queued connections do not count
			resp := run(exec, req)
			mu.Unlock()
			touch(activity)
			_ = json.NewEncoder(conn).Encode(resp)
			if opt.overMemoryCap() {
				// Stop accepting; the loop finishes the requests in hand,
				// releases the model, and the process exits.
				cancel()
			}
		}()
	}
}

// clientGone reports whether the client has closed its end of conn. A client
// sends its request and then only reads, so a read that finds the end of the
// stream, or an error, means it has gone; a read that times out means it is
// still waiting. The probe waits a millisecond at most.
func clientGone(conn net.Conn) bool {
	if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		return false
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	var buf [64]byte
	for {
		// Bytes here are the whitespace after the request; read past them.
		if _, err := conn.Read(buf[:]); err != nil {
			var ne net.Error
			return !errors.As(err, &ne) || !ne.Timeout()
		}
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
// server it returns ErrNoServer at once. queue is how long to wait for the
// server to start the request; past it the request is still queued and the
// error is ErrUnanswered, so the caller may run it. Once the server has
// started it, total bounds the wait for the answer, and giving up then is
// not ErrUnanswered: the server has the request. An answer with no start
// byte is a protocol error, also not ErrUnanswered, and the caller must
// not run the request again.
func Forward(sock string, req Request, connect, queue, total time.Duration) (Response, error) {
	if err := checkPrivateDir(filepath.Dir(sock)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Response{}, ErrNoServer
		}
		return Response{}, fmt.Errorf("%w: %w", ErrUnanswered, err)
	}
	conn, err := net.DialTimeout("unix", sock, connect)
	if err != nil {
		// Nothing there, nothing listening, or a leftover non-socket file:
		// in each case no server will answer.
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTSOCK) {
			return Response{}, ErrNoServer
		}
		return Response{}, fmt.Errorf("%w: connecting: %w", ErrUnanswered, err)
	}
	defer func() { _ = conn.Close() }()
	// A send that cannot finish never started a request. The ack wait then
	// gets its own queue budget, so a slow send does not eat into it.
	if err := conn.SetWriteDeadline(time.Now().Add(queue)); err != nil {
		return Response{}, fmt.Errorf("%w: sending: %w", ErrUnanswered, err)
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("%w: sending: %w", ErrUnanswered, err)
	}
	if err := readAck(conn, queue); err != nil {
		return Response{}, err
	}
	// The server has the request. Bound the answer by total, as before.
	if err := conn.SetDeadline(time.Now().Add(total)); err != nil {
		return Response{}, fmt.Errorf("the vaultmind server started the request, but the wait could not be bounded: %w", err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		// The connection ended after the start byte and before an answer.
		// Linux resets a Unix socket closed with unread bytes; macOS ends it.
		if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
			return Response{}, fmt.Errorf("%w: it closed the connection without answering", ErrUnanswered)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return Response{}, fmt.Errorf("the vaultmind server did not answer within %s", total)
		}
		return Response{}, fmt.Errorf("reading from the vaultmind server: %w", err)
	}
	return resp, nil
}

// readAck waits up to queue for the server to say it has started req. No
// byte in that time means the request is still queued: the caller may run
// it, and closing the connection (Forward's defer) makes the server skip it.
// A different byte is a server answering without starting, which is not
// ErrUnanswered: the caller must not run the request a second time.
func readAck(conn net.Conn, queue time.Duration) error {
	if err := conn.SetReadDeadline(time.Now().Add(queue)); err != nil {
		return fmt.Errorf("reading from the vaultmind server: %w", err)
	}
	var ack [1]byte
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) {
			return fmt.Errorf("%w: it closed the connection without answering", ErrUnanswered)
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return fmt.Errorf("%w: still queued after %s", ErrUnanswered, queue)
		}
		return fmt.Errorf("reading from the vaultmind server: %w", err)
	}
	if ack[0] != ackStarted {
		return fmt.Errorf("the vaultmind server answered without starting the request")
	}
	return nil
}
