package cmd

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// servedAsks runs two asks the way the resident server does: ExecuteWithIO
// for each, with the CLI reset between them. It does not call Main, so a
// release that belongs to the process must not run.
func servedAsks(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(resetCLIState)
	for i := 1; i <= 2; i++ {
		resetCLIState()
		var out, errw bytes.Buffer
		code := ExecuteWithIO([]string{"ask", "sharedrecalltoken", "--vault", dir, "--json"}, &out, &errw)
		require.Equalf(t, 0, code, "ask %d\nstdout: %s\nstderr: %s", i, out.String(), errw.String())
	}
}

// Two requests in one process are two served asks. The model loads on the
// first and stays loaded for the second. A release inside Execute closes it
// after the first ask, and this count becomes 2.
func TestExecuteWithIO_TwoAsksLoadTheEmbedderOnce(t *testing.T) {
	emb, loads := useLoadCountEmbedder(t)
	dir := proposeVault(t)
	seedMiniLMEmbeddings(t, dir, emb.vec)

	servedAsks(t, dir)

	assert.Equal(t, 1, *loads, "two served asks load the model once")
	assert.Zero(t, emb.closed, "a served ask does not release the process session")
}

// A one-shot process loads the model once and releases it when the command
// returns. Main is that entry: the binary's run calls it, and a served
// request does not.
func TestNotePropose_OneShotLoadsOnceAndReleasesAtExit(t *testing.T) {
	emb, loads := useLoadCountEmbedder(t)
	dir := proposeVault(t)
	seedMiniLMEmbeddings(t, dir, emb.vec)
	t.Cleanup(resetCLIState)

	code, text := runProcess(t, "note", "propose",
		"--vault", dir,
		"--question", "Where does the embedder session live?",
		"--finding", "The process keeps one session for ranking and for the draft.",
		"--close-band", "not-a-real-band",
		"--json")
	require.Equalf(t, 0, code, "output: %s", text)
	assert.Equal(t, 1, *loads, "note propose loads the model once")
	assert.Equal(t, 1, emb.closed, "a one-shot process releases the session when it exits")
}

// runProcess is one OS process: Main, which releases the embedder after the
// command returns. Output is captured so a failure is readable.
func runProcess(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	code := Main(args)
	require.NoError(t, w.Close())
	os.Stdout, os.Stderr = stdout, stderr
	<-done
	return code, buf.String()
}

// The server releases the session when it stops, on idle and on a signal.
// The model is loaded before the server starts, the way a served ask leaves
// it, and no request runs: the release is the shutdown path, not Execute.
func TestServeShutdown_ReleasesTheProcessEmbedder(t *testing.T) {
	emb, loads := useLoadCountEmbedder(t)
	dir := proposeVault(t)
	seedMiniLMEmbeddings(t, dir, emb.vec)

	tests := []struct {
		name string
		idle string
		stop func(t *testing.T, cancel context.CancelFunc, sock string)
	}{
		{name: "idle", idle: "100ms", stop: func(*testing.T, context.CancelFunc, string) {}},
		{name: "signal", idle: "1m", stop: func(t *testing.T, cancel context.CancelFunc, sock string) {
			t.Helper()
			require.Eventually(t, func() bool {
				c, err := net.Dial("unix", sock)
				if err == nil {
					_ = c.Close()
				}
				return err == nil
			}, 2*time.Second, 10*time.Millisecond)
			cancel()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closedBefore := emb.closed
			loadsBefore := *loads
			loadSharedSession(t, dir)
			require.Equal(t, loadsBefore+1, *loads)
			require.Equal(t, closedBefore, emb.closed)

			isolatedState(t)
			ctx, cancel := context.WithCancel(context.Background())
			sock, err := serveSocket()
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() { done <- serveUntilIdle(ctx, tt.idle, 0) }()
			tt.stop(t, cancel, sock)

			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				cancel()
				t.Fatal("the server did not exit")
			}
			cancel()
			assert.Equal(t, closedBefore+1, emb.closed, "shutdown releases the process session")
		})
	}
}

func loadSharedSession(t *testing.T, dir string) {
	t.Helper()
	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ret := query.BuildAutoRetrieverFull(context.Background(), db)
	require.NotNil(t, ret.Embedder)
	ret.Cleanup()
}
