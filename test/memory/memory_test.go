//go:build memory

// Package memory is the pre-release memory gate: it runs a real ORT build of
// vaultmind with the BGE-M3 model and fails when embedding or the warm server
// uses more memory than it was measured to need. 0.10.13 shipped a re-embed
// that took one process past 100 GB; unit tests had not looked at memory.
//
// Run with `task test:memory` (it builds the binary and sets
// VAULTMIND_MEMORY_BINARY). It needs the BGE-M3 model in the cache; the first
// run downloads it (about 2.2 GB).
package memory

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// peakBudget is the most memory a full embed or the warm server may peak at,
// model included. Measured 2026-10-16 on an ORT build: 2.8–3.2 GB for full
// embeds of real vaults, 2.2 GB for the server over 100 asks.
const peakBudget = 4 << 30

func binary(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("VAULTMIND_MEMORY_BINARY")
	require.NotEmpty(t, bin, "set VAULTMIND_MEMORY_BINARY to an ORT build of vaultmind (task test:memory does)")
	_, err := os.Stat(bin)
	require.NoError(t, err)
	return bin
}

// env isolates data and state but keeps the model cache, so the gate does not
// download the model on every run.
func env(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	return append(os.Environ(),
		"XDG_DATA_HOME="+filepath.Join(dir, "data"),
		"XDG_STATE_HOME="+filepath.Join(dir, "state"),
		// Turn off the embed pass's own memory stop (app.embed_max_rss_mb) so
		// the gate measures the pass instead of the stop.
		"VAULTMIND_APP_EMBED_MAX_RSS_MB=0",
	)
}

// peakOf reports a finished child's peak resident memory in bytes.
func peakOf(t *testing.T, ps *os.ProcessState) int64 {
	t.Helper()
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	require.True(t, ok, "no resource usage for the child on %s", runtime.GOOS)
	if runtime.GOOS == "darwin" {
		return ru.Maxrss
	}
	return ru.Maxrss * 1024
}

var words = strings.Fields(`memory retrieval note vault index embedding sparse dense token batch model
	query answer agent session research graph link activation consolidation sleep recall context
	window budget hybrid ranking keyword semantic vector cosine fusion reciprocal rank evidence
	decision source concept person project review measure threshold baseline replay plasticity
	learning forgetting curve spacing practice episode arc identity principle trust signal noise
	floor calibration latency throughput process server socket request response cache disk swap
	the of and to in is that for it as with on by this be are from at or an which was not but
	can have has more one all their when what there been also into than its other these such`)

// body is deterministic English-like text of about chars bytes.
func body(r *rand.Rand, chars int) string {
	var b strings.Builder
	for b.Len() < chars {
		b.WriteString(words[r.IntN(len(words))])
		if r.IntN(12) == 0 {
			b.WriteString(". ")
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// longNoteVault writes a vault shaped like the one that broke 0.10.13: notes
// past the token cap among many mid-length and short ones.
func longNoteVault(t *testing.T) (dir string, notes int) {
	t.Helper()
	dir = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n"), 0o644))
	r := rand.New(rand.NewPCG(1, 2))
	sizes := []struct{ count, chars int }{{10, 40000}, {25, 6000}, {35, 800}}
	for _, s := range sizes {
		for range s.count {
			id := fmt.Sprintf("concept-n%03d", notes)
			content := fmt.Sprintf("---\nid: %s\ntype: concept\ntitle: Note %d\n---\n%s\n", id, notes, body(r, s.chars))
			require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("n%03d.md", notes)), []byte(content), 0o644))
			notes++
		}
	}
	return dir, notes
}

func TestFullEmbed_PeakMemoryStaysInBudget(t *testing.T) {
	bin := binary(t)
	vault, notes := longNoteVault(t)
	cmd := exec.Command(bin, "index", "--vault", vault, "--full", "--embed")
	cmd.Env = env(t)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "index --embed failed:\n%s", out)
	require.Contains(t, string(out), fmt.Sprintf("Embedded %d notes (0 skipped, 0 errors) [model: bge-m3]", notes),
		"every note must be embedded with BGE-M3, or the peak measures something else:\n%s", out)
	peak := peakOf(t, cmd.ProcessState)
	t.Logf("full embed of %d notes: peak %d MiB (budget %d MiB)", notes, peak>>20, int64(peakBudget)>>20)
	require.LessOrEqual(t, peak, int64(peakBudget), "a full embed peaked at %d MiB", peak>>20)
}

func TestServe_PeakMemoryStaysInBudgetOver100Asks(t *testing.T) {
	bin := binary(t)
	vault, _ := longNoteVault(t)
	e := env(t)
	index := exec.Command(bin, "index", "--vault", vault, "--embed")
	index.Env = e
	out, err := index.CombinedOutput()
	require.NoError(t, err, "index --embed failed:\n%s", out)
	require.Contains(t, string(out), "[model: bge-m3]", "the vault must be embedded with BGE-M3:\n%s", out)

	// The server's own cap is off so the test measures it; the asks below
	// reach it because it listens on this binary's socket.
	server := exec.Command(bin, "serve", "--idle", "5m", "--max-rss-mb", "0")
	server.Env = e
	var serverOut strings.Builder
	server.Stdout, server.Stderr = &serverOut, &serverOut
	require.NoError(t, server.Start())
	t.Cleanup(func() { // a failed assertion below must not leave the server running
		_ = server.Process.Signal(syscall.SIGTERM)
		_, _ = server.Process.Wait()
	})
	t.Cleanup(func() { // runs first: report what the server was doing
		if !t.Failed() {
			return
		}
		if server.Process.Signal(syscall.Signal(0)) == nil {
			_ = server.Process.Signal(syscall.SIGQUIT) // Go prints every goroutine's stack
			time.Sleep(time.Second)
		}
		t.Logf("server output:\n%s", serverOut.String())
	})
	// The server listens before it loads the model, so a moment is enough. Its
	// socket path depends on the version string (a long one moves it, see
	// serve.SocketPath), so the test does not look for it. An ask that misses
	// the server fails below by name, so a slow start cannot pass unnoticed.
	time.Sleep(2 * time.Second)

	queries := []string{"memory consolidation during sleep", "how is a batch planned", "reciprocal rank fusion",
		"what does the noise floor calibrate", "why did the server grow", "spacing and forgetting"}
	for i := range 100 {
		ask := exec.Command(bin, "ask", queries[i%len(queries)]+fmt.Sprintf(" %d", i), "--vault", vault, "--budget", "1500")
		ask.Env = e
		out, err := ask.CombinedOutput()
		require.NoError(t, err, "ask %d failed:\n%s", i, out)
		// An ask the server answers never loads the model. One that fell back
		// to running here would, and the server's peak would not count it.
		require.Less(t, peakOf(t, ask.ProcessState), int64(1<<30), "ask %d loaded the model itself: it did not reach the server", i)
	}
	require.NoError(t, server.Process.Signal(syscall.Signal(0)), "the server exited during the asks")
	require.NoError(t, server.Process.Signal(syscall.SIGTERM))
	require.NoError(t, server.Wait())
	peak := peakOf(t, server.ProcessState)
	t.Logf("serve over 100 asks: peak %d MiB (budget %d MiB)", peak>>20, int64(peakBudget)>>20)
	require.Greater(t, peak, int64(1<<30), "the server never loaded the model, so it answered no asks")
	require.LessOrEqual(t, peak, int64(peakBudget), "the server peaked at %d MiB", peak>>20)
}
