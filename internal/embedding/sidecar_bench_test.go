//go:build dev

package embedding_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/require"
)

// TestSidecar_VsInProcess_Throughput measures BGE-M3 inference time for
// a representative batch of identity-vault note bodies, run through
// (a) the in-process ORT+CPU embedder vaultmind ships and
// (b) the Python+MPS sidecar.
//
// Set VAULTMIND_SIDECAR_BENCH=1 to enable. Set VAULTMIND_SIDECAR_PYTHON
// to the venv with torch+transformers (default: python3 on PATH).
func TestSidecar_VsInProcess_Throughput(t *testing.T) {
	if os.Getenv("VAULTMIND_SIDECAR_BENCH") == "" {
		t.Skip("set VAULTMIND_SIDECAR_BENCH=1 to run sidecar throughput bench")
	}

	// Read N note bodies from the identity vault as the workload
	const N = 8
	vaultDir := "../../vaultmind-identity"
	var texts []string
	walkRoots := []string{
		filepath.Join(vaultDir, "arcs"),
		filepath.Join(vaultDir, "references"),
	}
	for _, root := range walkRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if len(texts) >= N {
				break
			}
			if e.IsDir() {
				continue
			}
			body, err := os.ReadFile(filepath.Join(root, e.Name()))
			if err != nil {
				continue
			}
			texts = append(texts, string(body))
		}
	}
	require.GreaterOrEqual(t, len(texts), 4, "need at least 4 real note bodies for the bench")
	t.Logf("workload: %d notes, total %d chars", len(texts), totalChars(texts))

	// === In-process baseline (CPU) ===
	t.Log("--- in-process ORT+CPU ---")
	inProc, err := embedding.NewBGEM3Embedder(embedding.BGEM3Config())
	require.NoError(t, err)
	cpuStart := time.Now()
	cpuOut, err := inProc.EmbedFullBatch(context.Background(), texts)
	cpuElapsed := time.Since(cpuStart)
	_ = inProc.Close()
	require.NoError(t, err)
	require.Len(t, cpuOut, len(texts))
	t.Logf("  in-process: %v (%.2fs/note)", cpuElapsed, cpuElapsed.Seconds()/float64(len(texts)))

	// === Sidecar (MPS) ===
	t.Log("--- sidecar Python+MPS ---")
	python := os.Getenv("VAULTMIND_SIDECAR_PYTHON")
	if python == "" {
		python = "python3"
	}
	scriptPath, err := filepath.Abs(filepath.Join("sidecar", "embed_server.py"))
	require.NoError(t, err)

	sideStartupStart := time.Now()
	side, err := embedding.NewSidecarBGEM3(embedding.SidecarBGEM3Config{
		Python:     python,
		ScriptPath: scriptPath,
	})
	require.NoError(t, err)
	defer func() { _ = side.Close() }()
	t.Logf("  sidecar startup (incl. model load): %v device=%s", time.Since(sideStartupStart), side.Device())

	sideStart := time.Now()
	sideOut, err := side.EmbedFullBatch(context.Background(), texts)
	sideElapsed := time.Since(sideStart)
	require.NoError(t, err)
	require.Len(t, sideOut, len(texts))
	t.Logf("  sidecar inference (after startup): %v (%.2fs/note)", sideElapsed, sideElapsed.Seconds()/float64(len(texts)))

	speedup := float64(cpuElapsed) / float64(sideElapsed)
	t.Logf("--- sidecar/inproc inference speedup: %.2fx ---", speedup)

	// Sanity check outputs match in shape (not exact values — different
	// engines may differ at numerical precision)
	for i := range texts {
		require.Len(t, cpuOut[i].Dense, embedding.BGEM3Dims)
		require.Len(t, sideOut[i].Dense, embedding.BGEM3Dims)
		require.NotEmpty(t, cpuOut[i].Sparse, "in-process sparse should be non-empty")
		require.NotEmpty(t, sideOut[i].Sparse, "sidecar sparse should be non-empty")
		require.NotEmpty(t, cpuOut[i].ColBERT, "in-process colbert should be non-empty")
		require.NotEmpty(t, sideOut[i].ColBERT, "sidecar colbert should be non-empty")
	}
}

func totalChars(texts []string) int {
	n := 0
	for _, t := range texts {
		n += len(t)
	}
	return n
}
