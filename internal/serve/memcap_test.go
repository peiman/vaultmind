package serve_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/serve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A resident server that grows past its memory cap answers the request in
// hand and then exits, so the next ask starts a fresh process. Under the cap
// it keeps serving.
func TestServe_ExitsAfterARequestOverItsMemoryCap(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rss      int64
		wantExit bool
	}{
		{"over the cap", 5 << 30, true},
		{"under the cap", 1 << 30, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sock := socketIn(t)
			var checks atomic.Int32
			rss := func() int64 { checks.Add(1); return tc.rss }
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() {
				done <- serve.Serve(ctx, sock, func(serve.Request) serve.Response {
					return serve.Response{Stdout: []byte("ok")}
				}, time.Minute, serve.WithMemoryCap(4<<30, rss))
			}()
			require.Eventually(t, func() bool {
				c, err := net.Dial("unix", sock)
				if err == nil {
					_ = c.Close()
				}
				return err == nil
			}, 2*time.Second, 10*time.Millisecond)

			resp, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, 5*time.Second)
			require.NoError(t, err)
			assert.Equal(t, "ok", string(resp.Stdout), "the request in hand is still answered")

			select {
			case err := <-done:
				require.True(t, tc.wantExit, "the server exited under its cap")
				require.NoError(t, err)
			case <-time.After(500 * time.Millisecond):
				require.False(t, tc.wantExit, "the server kept running over its cap")
			}
			assert.GreaterOrEqual(t, checks.Load(), int32(1), "memory is checked after a request")
		})
	}
}

// Without the option the server never measures itself.
func TestServe_NoMemoryCapByDefault(t *testing.T) {
	sock := socketIn(t)
	start(t, sock, func(serve.Request) serve.Response { return serve.Response{} }, time.Minute)
	_, err := serve.Forward(sock, serve.Request{Args: []string{"ask"}}, time.Second, 5*time.Second)
	require.NoError(t, err)
}
