package cmd

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/serve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolatedState gives the test its own state dir, so the socket it serves on
// is its own, and stands in for starting a real server.
func isolatedState(t *testing.T) *int {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv(envNoServe, "")
	started := 0
	orig := startServer
	startServer = func() { started++ }
	t.Cleanup(func() { startServer = orig })
	return &started
}

func serveFake(t *testing.T, exec serve.Exec) {
	t.Helper()
	sock, err := serveSocket()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = serve.Serve(ctx, sock, exec, time.Minute); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	require.Eventually(t, func() bool {
		_, err := serve.Forward(sock, serve.Request{Args: []string{"ping"}}, time.Second, time.Second)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
}

// An ask goes to the running server, which answers with its exit code.
func TestTryServer_AnAskIsAnsweredByTheServer(t *testing.T) {
	isolatedState(t)
	var got serve.Request
	serveFake(t, func(r serve.Request) serve.Response { got = r; return serve.Response{Code: 4} })

	code, ok := tryServer([]string{"ask", "q", "--vault", "v"})
	require.True(t, ok)
	assert.Equal(t, 4, code)
	assert.Equal(t, []string{"ask", "q", "--vault", "v"}, got.Args)
	assert.NotEmpty(t, got.Dir)
	assert.Equal(t, "", got.Env[envNoServe], "the client's environment travels with the request")
}

// Only ask is forwarded, and VAULTMIND_NO_SERVE keeps even an ask here.
func TestTryServer_OnlyAskAndOnlyWhenOn(t *testing.T) {
	started := isolatedState(t)
	serveFake(t, func(serve.Request) serve.Response { return serve.Response{Code: 4} })

	_, ok := tryServer([]string{"search", "q"})
	assert.False(t, ok)
	_, ok = tryServer(nil)
	assert.False(t, ok)
	t.Setenv(envNoServe, "1")
	_, ok = tryServer([]string{"ask", "q"})
	assert.False(t, ok)
	assert.Zero(t, *started)
}

// With no server the ask runs here, and a server is started for the next.
func TestTryServer_NoServerRunsHereAndStartsOne(t *testing.T) {
	started := isolatedState(t)
	_, ok := tryServer([]string{"ask", "q"})
	assert.False(t, ok)
	assert.Equal(t, 1, *started)
}

// A server that closes the connection without answering never ran the ask
// to the end, so it runs here. One is listening, so no second is started.
func TestTryServer_AServerThatDiesBeforeAnsweringRunsHere(t *testing.T) {
	started := isolatedState(t)
	sock, err := serveSocket()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(sock), 0o700))
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		if c, err := ln.Accept(); err == nil {
			_, _ = c.Read(make([]byte, 1<<16))
			_ = c.Close()
		}
	}()

	_, ok := tryServer([]string{"ask", "q"})
	assert.False(t, ok)
	assert.Zero(t, *started)
}

// The server serves until idle, and a second one for the same binary steps
// aside without an error.
func TestServeUntilIdle(t *testing.T) {
	isolatedState(t)
	serveFake(t, func(serve.Request) serve.Response { return serve.Response{} })
	assert.NoError(t, serveUntilIdle(context.Background(), "1m", 0), "another server already answers")

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	startAt := time.Now()
	require.NoError(t, serveUntilIdle(context.Background(), "100ms", 0))
	assert.Less(t, time.Since(startAt), 5*time.Second)

	assert.Error(t, serveUntilIdle(context.Background(), "soon", 0))
	assert.Error(t, serveUntilIdle(context.Background(), "0s", 0))
}

// An ask the server has taken is never run a second time here, however long
// it takes. Rerunning it left two copies of one ask competing for the CPU and
// two ask events for one query (#406). Even past the client's safety cap the
// client reports an error rather than rerunning, and starts no second server.
func TestTryServer_ASlowServedAskRunsOnlyOnce(t *testing.T) {
	started := isolatedState(t)
	orig := serveRequestTimeout
	serveRequestTimeout = 100 * time.Millisecond
	t.Cleanup(func() { serveRequestTimeout = orig })
	var runs atomic.Int32
	serveFake(t, func(r serve.Request) serve.Response {
		if r.Args[0] == "ask" {
			runs.Add(1)
			time.Sleep(400 * time.Millisecond)
		}
		return serve.Response{}
	})

	code, ok := tryServer([]string{"ask", "q"})
	assert.True(t, ok, "the ask was rerun in this process")
	assert.NotZero(t, code, "an ask the server never answered is not a success")
	assert.Equal(t, int32(1), runs.Load())
	assert.Zero(t, *started)
}

// The client's cap on a served ask is a safety net for a wedged server, not a
// latency budget: callers such as the hooks bound their own wait. A short cap
// abandons slow asks that are still running (#406).
func TestServeRequestTimeout_IsALongSafetyCap(t *testing.T) {
	assert.GreaterOrEqual(t, serveRequestTimeout, time.Minute)
}

// Main answers an ask from the server, with the server's exit code.
func TestMain_AnAskGoesToTheServer(t *testing.T) {
	isolatedState(t)
	serveFake(t, func(serve.Request) serve.Response { return serve.Response{Code: 5} })
	assert.Equal(t, 5, Main([]string{"ask", "q"}))
}

// The spawned server runs `<exe> serve` from the temp dir, detached.
func TestSpawnServer_RunsServeFromTheTempDir(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	exe := filepath.Join(dir, "fake-vaultmind")
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\necho \"$1 $(pwd -P)\" > "+marker+"\n"), 0o700))

	spawnServer(exe)
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 5*time.Second, 20*time.Millisecond)
	tmp, err := filepath.EvalSymlinks(os.TempDir())
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		b, _ := os.ReadFile(marker)
		return strings.TrimSpace(string(b)) == "serve "+tmp
	}, 2*time.Second, 20*time.Millisecond)

	spawnServer(filepath.Join(dir, "missing")) // a binary that can't start is no error
}

// The serve command reads --idle and rejects a value that isn't a duration.
func TestServeCommand_RejectsABadIdle(t *testing.T) {
	cleanupCLIState(t)
	isolatedState(t)
	resetCLIState()
	var out, errw bytes.Buffer
	assert.Equal(t, 1, ExecuteWithIO([]string{"serve", "--idle", "soon"}, &out, &errw))
	assert.Contains(t, errw.String(), `--idle "soon"`)
}

// In JSON mode a failed command's error is an envelope on stdout, as main
// reported it before the server existed.
func TestExecuteWithIO_JSONErrorIsAnEnvelope(t *testing.T) {
	cleanupCLIState(t)
	isolatedState(t)
	resetCLIState()
	var out, errw bytes.Buffer
	assert.Equal(t, 1, ExecuteWithIO([]string{"serve", "--idle", "soon", "--output-format", "json"}, &out, &errw))
	assert.Contains(t, out.String(), `"status": "error"`)
	assert.Contains(t, out.String(), `--idle`)
}
