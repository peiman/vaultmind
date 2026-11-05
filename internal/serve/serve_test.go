package serve_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/serve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// socketIn returns a socket path short enough for a Unix socket (macOS
// allows 104 bytes; t.TempDir paths on macOS are long).
func socketIn(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "vms")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func start(t *testing.T, sock string, exec serve.Exec, idle time.Duration) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- serve.Serve(ctx, sock, exec, idle) }()
	require.Eventually(t, func() bool {
		c, err := net.Dial("unix", sock)
		if err == nil {
			_ = c.Close()
		}
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
	return done
}

// A request carries the CLI's arguments, directory and environment; the
// answer carries what the command printed and its exit code.
func TestForward_RoundTrip(t *testing.T) {
	sock := socketIn(t)
	start(t, sock, func(r serve.Request) serve.Response {
		return serve.Response{Stdout: []byte("out:" + r.Args[0] + ":" + r.Dir + ":" + r.Env["K"]), Stderr: []byte("err"), Code: 3}
	}, time.Minute)

	resp, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}, Dir: "/d", Env: map[string]string{"K": "v"}}, time.Second, time.Second, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "out:ask:/d:v", string(resp.Stdout))
	assert.Equal(t, "err", string(resp.Stderr))
	assert.Equal(t, 3, resp.Code)
}

// Requests run one at a time: the CLI's flags and the process's directory
// and environment are shared, so two at once would mix them.
func TestServe_RunsOneRequestAtATime(t *testing.T) {
	sock := socketIn(t)
	var running, overlap atomic.Int32
	start(t, sock, func(serve.Request) serve.Response {
		if running.Add(1) > 1 {
			overlap.Store(1)
		}
		time.Sleep(50 * time.Millisecond)
		running.Add(-1)
		return serve.Response{}
	}, time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 5*time.Second)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	assert.Zero(t, overlap.Load())
}

// With no server, Forward says so at once, so the caller runs the command
// itself and loses no time.
func TestForward_NoServerIsImmediate(t *testing.T) {
	startAt := time.Now()
	_, err := serve.Forward(socketIn(t), serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 5*time.Second)
	assert.ErrorIs(t, err, serve.ErrNoServer)
	assert.Less(t, time.Since(startAt), 500*time.Millisecond)
}

// A server slower than the caller's budget is abandoned, not waited on.
func TestForward_GivesUpAfterItsBudget(t *testing.T) {
	sock := socketIn(t)
	start(t, sock, func(serve.Request) serve.Response {
		time.Sleep(2 * time.Second)
		return serve.Response{}
	}, time.Minute)
	_, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 200*time.Millisecond)
	require.Error(t, err)
	assert.NotErrorIs(t, err, serve.ErrNoServer)
	assert.NotErrorIs(t, err, serve.ErrUnanswered, "a server that has started the request still has it")
}

// A request whose client has gone while it waited its turn is not run: no
// one is left to read the answer, and running it would only delay the asks
// queued behind it. A hook that times out kills its client this way (#406).
func TestServe_SkipsAQueuedRequestWhoseClientHasGone(t *testing.T) {
	sock := socketIn(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var abandonedRuns atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- serve.Serve(ctx, sock, func(r serve.Request) serve.Response {
			switch r.Args[0] {
			case "hold":
				close(entered)
				<-release
			case "abandoned":
				abandonedRuns.Add(1)
			}
			return serve.Response{}
		}, time.Minute)
	}()
	require.Eventually(t, func() bool {
		_, err := serve.Forward(sock, serve.Request{Args: []string{"ping"}}, time.Second, time.Second, time.Second)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	held := make(chan error, 1)
	go func() {
		_, err := serve.Forward(sock, serve.Request{Args: []string{"hold"}}, time.Second, time.Second, 5*time.Second)
		held <- err
	}()
	<-entered
	// Queued behind "hold"; the client gives up and closes its connection.
	_, err := serve.Forward(sock, serve.Request{Args: []string{"abandoned"}}, time.Second, 100*time.Millisecond, 5*time.Second)
	require.Error(t, err)
	close(release)
	require.NoError(t, <-held)

	// Serve returns only after every accepted request is handled.
	cancel()
	require.NoError(t, <-done)
	assert.Zero(t, abandonedRuns.Load(), "a request whose client had gone was run")
}

// A server that closes the connection without a word of answer refused the
// request or died first: Forward says it went unanswered, so the caller may
// run it itself. A server that is still working is not that (#406).
func TestForward_AClosedConnectionWithNoAnswerIsUnanswered(t *testing.T) {
	sock := socketIn(t)
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Read(make([]byte, 4096))
			_ = c.Close()
		}
	}()
	_, err = serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 5*time.Second)
	assert.ErrorIs(t, err, serve.ErrUnanswered)

	slow := socketIn(t)
	start(t, slow, func(serve.Request) serve.Response {
		time.Sleep(500 * time.Millisecond)
		return serve.Response{}
	}, time.Minute)
	_, err = serve.Forward(slow, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 100*time.Millisecond)
	require.Error(t, err)
	assert.NotErrorIs(t, err, serve.ErrUnanswered, "a server past the cap still has the request")
	assert.NotErrorIs(t, err, serve.ErrNoServer)
}

// The server stamps each request when it reads it, so the time an ask waits
// for its turn counts in how long it took.
func TestServe_StampsWhenItReadARequest(t *testing.T) {
	sock := socketIn(t)
	got := make(chan time.Time, 1)
	start(t, sock, func(r serve.Request) serve.Response {
		if r.Args[0] == "ask" {
			got <- r.Received
		}
		return serve.Response{}
	}, time.Minute)
	before := time.Now()
	_, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 5*time.Second)
	require.NoError(t, err)
	received := <-got
	assert.False(t, received.Before(before))
	assert.False(t, received.After(time.Now()))
}

// An idle server exits and removes its socket, releasing what it holds.
func TestServe_ExitsWhenIdle(t *testing.T) {
	sock := socketIn(t)
	done := start(t, sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Second)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(12 * time.Second):
		t.Fatal("the server did not exit when idle")
	}
	assert.NoFileExists(t, sock)
}

// A second server for the same socket exits quietly when the first answers;
// a stale socket file left by a crashed server is replaced.
func TestServe_OneServerPerSocket(t *testing.T) {
	sock := socketIn(t)
	start(t, sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute)
	err := serve.Serve(context.Background(), sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute)
	assert.ErrorIs(t, err, serve.ErrAlreadyServing)

	stale := socketIn(t)
	require.NoError(t, os.WriteFile(stale, nil, 0o600))
	start(t, stale, func(serve.Request) serve.Response { return serve.Response{Code: 7} }, time.Minute)
	resp, err := serve.Forward(stale, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 7, resp.Code)
}

// The socket is the user's alone.
func TestServe_SocketIsPrivate(t *testing.T) {
	sock := socketIn(t)
	start(t, sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute)
	info, err := os.Stat(sock)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// The socket name carries the version, so two installed versions each have
// their own server, and stays within the Unix socket length limit.
func TestSocketPath_PerVersionAndShort(t *testing.T) {
	a := serve.SocketPath("/state", "v0.10.4")
	b := serve.SocketPath("/state", "v0.10.5")
	assert.NotEqual(t, a, b)
	long := serve.SocketPath("/"+string(make([]byte, 0))+filepath.Join("very", "long", "state", "directory", "path", "that", "keeps", "going", "and", "going", "for", "a", "while", "longer"), "v0.10.4-12-gabcdef0-dirty")
	assert.LessOrEqual(t, len(long), 100)
}

// A request that panics fails alone; the server keeps answering.
func TestServe_APanicFailsOnlyItsRequest(t *testing.T) {
	sock := socketIn(t)
	start(t, sock, func(r serve.Request) serve.Response {
		if r.Args[0] == "boom" {
			panic("boom")
		}
		return serve.Response{Code: 2}
	}, time.Minute)

	resp, err := serve.Forward(sock, serve.Request{Args: []string{"boom"}}, time.Second, time.Second, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 70, resp.Code)
	assert.Contains(t, string(resp.Stderr), "boom")
	resp, err = serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Code)
}

// A version string with characters a file name shouldn't carry still names
// its own socket.
func TestSocketPath_OddVersionCharacters(t *testing.T) {
	p := serve.SocketPath("/state", "v1.0.0+meta/x y")
	assert.Equal(t, "/state/serve/serve-v1.0.0_meta_x_y.sock", p)
}

// A socket directory another user could reach is refused by both sides: the
// server won't listen there and the client won't send its environment there.
func TestServe_RefusesADirOthersCanReach(t *testing.T) {
	sock := socketIn(t)
	require.NoError(t, os.Chmod(filepath.Dir(sock), 0o777))
	err := serve.Serve(context.Background(), sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute)
	assert.ErrorIs(t, err, serve.ErrUnsafeDir)

	_, err = serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, time.Second)
	assert.ErrorIs(t, err, serve.ErrUnsafeDir)

	// A missing directory just means no server yet.
	_, err = serve.Forward(filepath.Join(filepath.Dir(sock), "gone", "s.sock"), serve.Request{}, time.Second, time.Second, time.Second)
	assert.ErrorIs(t, err, serve.ErrNoServer)
}

// A signalled server finishes the request it is running before it returns
// (after which the process exits, so a request still running would be cut).
func TestServe_FinishesInFlightRequestsOnShutdown(t *testing.T) {
	sock := socketIn(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	entered := make(chan struct{})
	var finished atomic.Bool
	go func() {
		done <- serve.Serve(ctx, sock, func(r serve.Request) serve.Response {
			if r.Args[0] == "slow" {
				close(entered)
				time.Sleep(300 * time.Millisecond)
				finished.Store(true)
			}
			return serve.Response{Code: 9}
		}, time.Minute)
	}()
	require.Eventually(t, func() bool {
		_, err := serve.Forward(sock, serve.Request{Args: []string{"x"}}, time.Second, time.Second, time.Second)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	got := make(chan serve.Response, 1)
	go func() {
		resp, _ := serve.Forward(sock, serve.Request{Args: []string{"slow"}}, time.Second, time.Second, 5*time.Second)
		got <- resp
	}()
	<-entered
	cancel()
	require.NoError(t, <-done)
	assert.True(t, finished.Load(), "Serve returned while a request was still running")
	assert.Equal(t, 9, (<-got).Code)
}

// Shutdown runs once when the server stops, on idle and on cancel, and not
// when this process never takes the socket.
func TestServe_ShutdownRunsWhenTheServerStops(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		assert.Equal(t, int32(1), shutdownCalls(t, time.Second, func(context.CancelFunc) {}))
	})
	t.Run("signal", func(t *testing.T) {
		assert.Equal(t, int32(1), shutdownCalls(t, time.Minute, func(cancel context.CancelFunc) { cancel() }))
	})
	t.Run("already serving", func(t *testing.T) {
		sock := socketIn(t)
		start(t, sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute)
		var calls atomic.Int32
		err := serve.Serve(context.Background(), sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute, serve.WithShutdown(func() { calls.Add(1) }))
		assert.ErrorIs(t, err, serve.ErrAlreadyServing)
		assert.Zero(t, calls.Load())
	})
	t.Run("unsafe directory", func(t *testing.T) {
		sock := socketIn(t)
		require.NoError(t, os.Chmod(filepath.Dir(sock), 0o777))
		var calls atomic.Int32
		err := serve.Serve(context.Background(), sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute, serve.WithShutdown(func() { calls.Add(1) }))
		assert.ErrorIs(t, err, serve.ErrUnsafeDir)
		assert.Zero(t, calls.Load())
	})
}

// A request that never finishes used to keep the server up for good. Every
// accepted connection reset the idle timer, including ones that only waited
// behind the stuck request, so the timer never fired and later asks had
// nowhere else to go (#421). New connections must not keep a wedged server
// alive, and once it has dropped its listener a new server can take the
// socket while the wedged process is still running.
func TestServe_AWedgedRequestGoesIdleWhileConnectionsArrive(t *testing.T) {
	sock := socketIn(t)
	const idle = time.Second
	started := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve.Serve(ctx, sock, func(r serve.Request) serve.Response {
			if len(r.Args) > 0 && r.Args[0] == "wedge" {
				close(started)
				<-release
			}
			return serve.Response{}
		}, idle)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	// A finished request arms the idle timer from now, so the wait below is
	// the wedged request's own idle, not the time the server took to start.
	require.Eventually(t, func() bool {
		_, err := serve.Forward(sock, serve.Request{Args: []string{"ping"}}, time.Second, time.Second, time.Second)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	go func() {
		_, _ = serve.Forward(sock, serve.Request{Args: []string{"wedge"}}, time.Second, time.Second, time.Minute)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the wedged request did not start")
	}

	var arrived int
	// One second of idle, so 12 s is twelve times that. A server that
	// refreshes its idle timer on accept stays up the whole time; one that
	// does not drops the listener, and with it the socket file, after one
	// idle period. The margin is wide on purpose: a loaded machine runs this
	// suite slowly (#415).
	gone := assert.Eventually(t, func() bool {
		c, err := net.DialTimeout("unix", sock, 100*time.Millisecond)
		if err != nil {
			_, statErr := os.Stat(sock)
			return os.IsNotExist(statErr)
		}
		arrived++
		_ = c.Close()
		return false
	}, 12*time.Second, 15*time.Millisecond)
	require.True(t, gone, "the wedged server kept its socket while connections arrived (%d accepted)", arrived)
	assert.Greater(t, arrived, 0, "no connection arrived while the request was wedged")

	select {
	case err := <-done:
		t.Fatalf("the wedged server process exited: %v", err)
	default:
	}
	_, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, time.Second)
	assert.ErrorIs(t, err, serve.ErrNoServer)

	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	go func() {
		_ = serve.Serve(ctx2, sock, func(serve.Request) serve.Response {
			return serve.Response{Code: 5}
		}, time.Minute)
	}()
	require.Eventually(t, func() bool {
		resp, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, time.Second)
		return err == nil && resp.Code == 5
	}, 2*time.Second, 10*time.Millisecond, "a new server could not take the socket the wedged one had closed")
}

// A wedged server goes idle and a replacement binds the same path. When the
// wedge finally ends, the first server must not unlink the replacement.
func TestServe_AFinishedWedgeLeavesTheReplacementSocket(t *testing.T) {
	sock := socketIn(t)
	const idle = time.Second
	started := make(chan struct{})
	release := make(chan struct{})
	ctxA, cancelA := context.WithCancel(context.Background())
	aDone := make(chan error, 1)
	go func() {
		aDone <- serve.Serve(ctxA, sock, func(r serve.Request) serve.Response {
			if len(r.Args) > 0 && r.Args[0] == "wedge" {
				close(started)
				<-release
			}
			return serve.Response{}
		}, idle)
	}()
	t.Cleanup(func() {
		cancelA()
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-aDone:
		case <-time.After(5 * time.Second):
		}
	})
	require.Eventually(t, func() bool {
		_, err := serve.Forward(sock, serve.Request{Args: []string{"ping"}}, time.Second, time.Second, time.Second)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	go func() {
		_, _ = serve.Forward(sock, serve.Request{Args: []string{"wedge"}}, time.Second, time.Second, time.Minute)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the wedged request did not start")
	}

	// Idle closes A's listener and unlinks A's socket. Twelve times the idle
	// period, for a loaded machine (#415). B must bind only after that file
	// is gone, or it finds A still listening.
	require.Eventually(t, func() bool {
		_, err := os.Stat(sock)
		return os.IsNotExist(err)
	}, 12*time.Second, 15*time.Millisecond, "the wedged server did not release its socket")

	ctxB, cancelB := context.WithCancel(context.Background())
	bDone := make(chan error, 1)
	go func() {
		bDone <- serve.Serve(ctxB, sock, func(serve.Request) serve.Response {
			return serve.Response{Code: 7}
		}, time.Minute)
	}()
	t.Cleanup(func() {
		cancelB()
		select {
		case <-bDone:
		case <-time.After(5 * time.Second):
		}
	})
	require.Eventually(t, func() bool {
		resp, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, time.Second)
		return err == nil && resp.Code == 7
	}, 2*time.Second, 10*time.Millisecond, "the replacement server did not answer")

	close(release)
	select {
	case err := <-aDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the wedged server did not return after its request was released")
	}

	_, err := os.Stat(sock)
	require.NoError(t, err, "the finished server removed the replacement's socket")
	resp, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, time.Second)
	require.NoError(t, err)
	assert.Equal(t, 7, resp.Code)
}

// An ask queued behind one that never finishes must give up at the queue cap
// and leave that ask to run only on the server. The client then runs its own
// ask, and the server skips the one whose client has gone, so the two are
// not two copies of one ask (#421).
func TestForward_ARequestQueuedBehindAWedgedOneIsUnanswered(t *testing.T) {
	sock := socketIn(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var ranA, ranB atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve.Serve(ctx, sock, func(r serve.Request) serve.Response {
			switch r.Args[0] {
			case "A":
				ranA.Add(1)
				close(started)
				<-release
			case "B":
				ranB.Add(1)
			}
			return serve.Response{}
		}, time.Minute)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
	})
	require.Eventually(t, func() bool {
		_, err := serve.Forward(sock, serve.Request{Args: []string{"ping"}}, time.Second, time.Second, time.Second)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	go func() {
		_, _ = serve.Forward(sock, serve.Request{Args: []string{"A"}}, time.Second, time.Second, time.Minute)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the wedged request did not start")
	}

	const queue = 100 * time.Millisecond
	begin := time.Now()
	_, err := serve.Forward(sock, serve.Request{Args: []string{"B"}}, time.Second, queue, 3*time.Second)
	elapsed := time.Since(begin)
	assert.ErrorIs(t, err, serve.ErrUnanswered)
	assert.GreaterOrEqual(t, elapsed, queue/2, "gave up without waiting for the queue")
	assert.Less(t, elapsed, 20*queue, "waited %s; the queue cap is %s", elapsed, queue)
	if t.Failed() {
		return
	}

	// Releasing A lets the server reach B. Serve returns only after every
	// accepted handler finishes, so B has been skipped or run by then.
	close(release)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not finish after the wedged request was released")
	}
	assert.Equal(t, int32(1), ranA.Load())
	assert.Zero(t, ranB.Load(), "the ask the client abandoned in the queue was run")
}

// A server that sends an answer without first saying it has started the
// request is speaking an older protocol. That is not "still queued": the
// caller must not run the ask again (#421).
func TestForward_AnAnswerWithoutAnAckIsNotUnanswered(t *testing.T) {
	sock := socketIn(t)
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		var req serve.Request
		if json.NewDecoder(c).Decode(&req) != nil {
			return
		}
		_ = json.NewEncoder(c).Encode(serve.Response{Stdout: []byte("no-ack"), Code: 3})
	}()

	_, err = serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second, time.Second)
	require.Error(t, err)
	assert.NotErrorIs(t, err, serve.ErrUnanswered)
	assert.NotErrorIs(t, err, serve.ErrNoServer)
}

func shutdownCalls(t *testing.T, idle time.Duration, stop func(context.CancelFunc)) int32 {
	t.Helper()
	sock := socketIn(t)
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve.Serve(ctx, sock, func(serve.Request) serve.Response { return serve.Response{} }, idle, serve.WithShutdown(func() { calls.Add(1) }))
	}()
	require.Eventually(t, func() bool {
		c, err := net.Dial("unix", sock)
		if err == nil {
			_ = c.Close()
		}
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
	stop(cancel)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("the server did not exit")
	}
	cancel()
	return calls.Load()
}
