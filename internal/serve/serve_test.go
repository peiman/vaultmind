package serve_test

import (
	"context"
	"errors"
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

	resp, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}, Dir: "/d", Env: map[string]string{"K": "v"}}, time.Second, 5*time.Second)
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
			_, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, 5*time.Second)
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
	_, err := serve.Forward(socketIn(t), serve.Request{Args: []string{"ask"}}, time.Second, 5*time.Second)
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
	_, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, 200*time.Millisecond)
	require.Error(t, err)
	assert.False(t, errors.Is(err, serve.ErrNoServer))
}

// An idle server exits and removes its socket, releasing what it holds.
func TestServe_ExitsWhenIdle(t *testing.T) {
	sock := socketIn(t)
	done := start(t, sock, func(serve.Request) serve.Response { return serve.Response{} }, 150*time.Millisecond)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
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
	resp, err := serve.Forward(stale, serve.Request{Args: []string{"ask"}}, time.Second, 5*time.Second)
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

	resp, err := serve.Forward(sock, serve.Request{Args: []string{"boom"}}, time.Second, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 70, resp.Code)
	assert.Contains(t, string(resp.Stderr), "boom")
	resp, err = serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, 5*time.Second)
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

	_, err = serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, time.Second)
	assert.ErrorIs(t, err, serve.ErrUnsafeDir)

	// A missing directory just means no server yet.
	_, err = serve.Forward(filepath.Join(filepath.Dir(sock), "gone", "s.sock"), serve.Request{}, time.Second, time.Second)
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
		_, err := serve.Forward(sock, serve.Request{Args: []string{"x"}}, time.Second, time.Second)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)

	got := make(chan serve.Response, 1)
	go func() {
		resp, _ := serve.Forward(sock, serve.Request{Args: []string{"slow"}}, time.Second, 5*time.Second)
		got <- resp
	}()
	<-entered
	cancel()
	require.NoError(t, <-done)
	assert.True(t, finished.Load(), "Serve returned while a request was still running")
	assert.Equal(t, 9, (<-got).Code)
}
