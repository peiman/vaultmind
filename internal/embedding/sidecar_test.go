package embedding

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMain doubles as the fake BGE-M3 sidecar. When GO_WANT_SIDECAR_HELPER=1 is
// set in the environment, the re-executed test binary speaks the sidecar's JSON
// line protocol instead of running tests. This lets us exercise the real
// NewSidecarBGEM3 plumbing (StdinPipe/StdoutPipe/StderrPipe + the request/response
// round-trip) against a controllable process, with no Python, torch, or model —
// so the deadlock regression is guarded in the default `task test`, not just under
// the `//go:build dev` tag the live-model tests use.
func TestMain(m *testing.M) {
	switch os.Getenv("GO_WANT_SIDECAR_HELPER") {
	case "flood":
		runFakeFloodingSidecar()
		return
	case "startup_fail":
		// Write to stderr, then die before emitting a ready line — exercises the
		// startup-error path that must surface the drained stderr tail.
		fmt.Fprintln(os.Stderr, "fatal: could not load model weights")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// runFakeFloodingSidecar emits the ready line, then for every request writes a
// large burst to stderr *before* writing the stdout response. That ordering is
// the crux of the regression: if the parent never drains stderr, the OS pipe
// buffer (~64KB on macOS) fills, this process blocks in the stderr Write and so
// never writes the response, and the parent blocks forever reading stdout.
func runFakeFloodingSidecar() {
	out := bufio.NewWriter(os.Stdout)
	fmt.Fprint(out, `{"ready":true,"device":"cpu"}`+"\n")
	_ = out.Flush()

	// 256KB >> any single-pipe buffer, so the stderr Write blocks hard until
	// something on the other end drains it.
	flood := bytes.Repeat([]byte("bge-m3 tokenizer warning: sequence length longer than maximum\n"), 4096)

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 64<<20) // tolerate large request lines
	for in.Scan() {
		var req struct {
			Texts []string `json:"texts"`
		}
		if err := json.Unmarshal(in.Bytes(), &req); err != nil {
			return
		}
		n := len(req.Texts)

		// Flood stderr first — the deadlock trigger.
		_, _ = os.Stderr.Write(flood)

		// Minimal valid response: EmbedFullBatch indexes resp.Dense[i],
		// resp.Sparse[i], resp.ColBERT[i] for every i < len(texts), so all
		// three arrays must be length n and sparse/colbert non-empty.
		resp := map[string]any{
			"dense":   make([][]float32, n),
			"sparse":  make([]map[string]float32, n),
			"colbert": make([][][]float32, n),
		}
		dense := resp["dense"].([][]float32)
		sparse := resp["sparse"].([]map[string]float32)
		colbert := resp["colbert"].([][][]float32)
		for i := 0; i < n; i++ {
			dense[i] = []float32{0.1, 0.2, 0.3}
			sparse[i] = map[string]float32{"1": 0.5}
			colbert[i] = [][]float32{{0.1, 0.2, 0.3}}
		}
		b, _ := json.Marshal(resp)
		out.Write(b)
		out.WriteByte('\n')
		_ = out.Flush()
	}
}

// TestSidecar_StderrFloodDoesNotDeadlock reproduces the 24/42 embed wedge: a
// sidecar that emits a burst to stderr before responding must NOT hang the
// caller. Before the stderr-drain fix, EmbedFullBatch blocks forever here and the
// test fails on the timeout; after it, the batch completes.
func TestSidecar_StderrFloodDoesNotDeadlock(t *testing.T) {
	t.Setenv("GO_WANT_SIDECAR_HELPER", "flood")

	emb, err := NewSidecarBGEM3(SidecarBGEM3Config{
		Python:     os.Args[0], // re-exec this test binary as the fake sidecar
		ScriptPath: os.Args[0], // any real, stat-able path; the fake ignores it
	})
	require.NoError(t, err, "fake sidecar should start and signal ready")
	defer func() { _ = emb.Close() }()

	done := make(chan error, 1)
	go func() {
		_, e := emb.EmbedFullBatch(context.Background(), []string{"alpha", "beta", "gamma"})
		done <- e
	}()

	select {
	case e := <-done:
		require.NoError(t, e, "batch should embed cleanly once stderr is drained")
	case <-time.After(15 * time.Second):
		// Unblock the wedged reader so cleanup and the -timeout harness don't
		// hang for a full minute: killing the process EOFs the stdout read the
		// call is stuck on, releasing the mutex Close() needs.
		if emb.cmd != nil && emb.cmd.Process != nil {
			_ = emb.cmd.Process.Kill()
		}
		t.Fatal("EmbedFullBatch deadlocked: the sidecar's stderr pipe filled and was never drained (the 24/42 wedge)")
	}
}

// TestSidecar_StartupFailure_SurfacesStderrTail covers the startup-error path:
// when the sidecar exits before signaling ready, NewSidecarBGEM3 must fail with
// an error that carries the drained stderr tail. That only works if the drain
// goroutine captured stderr and Close joined it before the tail is read.
func TestSidecar_StartupFailure_SurfacesStderrTail(t *testing.T) {
	t.Setenv("GO_WANT_SIDECAR_HELPER", "startup_fail")

	_, err := NewSidecarBGEM3(SidecarBGEM3Config{
		Python:     os.Args[0],
		ScriptPath: os.Args[0],
	})
	require.Error(t, err, "startup must fail when the sidecar exits before ready")
	require.Contains(t, err.Error(), "could not load model weights",
		"the startup error should surface the drained stderr tail")
}
