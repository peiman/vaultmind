//go:build dev

package embedding_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/require"
)

// TestSidecar_NumericalEquivalence_VsInProcess verifies that the sidecar
// produces embeddings close enough to the in-process ORT+CPU path to be
// retrieval-equivalent. Different engines + different precision (fp16 on
// MPS vs fp32 on CPU) introduce drift; the question is whether that
// drift is small enough that retrieval rankings stay consistent.
//
// Pass criteria:
//   - Dense cosine similarity >= 0.98 (tight — same query through dense
//     retrieval should rank notes nearly identically)
//   - Sparse top-K token overlap >= 0.7 (sparse weights are noisier,
//     small absolute differences are fine if dominant terms agree)
//   - ColBERT per-token cosine similarity >= 0.95 (looser — per-token
//     drift compounds in MaxSim; verify shape and rough alignment)
//
// Set VAULTMIND_SIDECAR_BENCH=1 + VAULTMIND_SIDECAR_PYTHON to enable.
func TestSidecar_NumericalEquivalence_VsInProcess(t *testing.T) {
	if os.Getenv("VAULTMIND_SIDECAR_BENCH") == "" {
		t.Skip("set VAULTMIND_SIDECAR_BENCH=1 to run sidecar equivalence test")
	}

	// Use 4 short, varied texts — enough to surface drift, fast enough
	// to keep the test cheap.
	texts := []string{
		"spreading activation is a method for searching associative networks",
		"Hebbian learning strengthens connections between co-firing neurons",
		"the workhorse agent gained partner-shaped memory across sessions",
		"reciprocal rank fusion combines retrievers via 1/(K+rank)",
	}

	// In-process baseline
	inProc, err := embedding.NewBGEM3Embedder(embedding.BGEM3Config())
	require.NoError(t, err)
	cpuOut, err := inProc.EmbedFullBatch(context.Background(), texts)
	_ = inProc.Close()
	require.NoError(t, err)

	// Sidecar
	python := os.Getenv("VAULTMIND_SIDECAR_PYTHON")
	if python == "" {
		python = "python3"
	}
	scriptPath, err := filepath.Abs(filepath.Join("sidecar", "embed_server.py"))
	require.NoError(t, err)
	side, err := embedding.NewSidecarBGEM3(embedding.SidecarBGEM3Config{
		Python:     python,
		ScriptPath: scriptPath,
	})
	require.NoError(t, err)
	defer func() { _ = side.Close() }()
	sideOut, err := side.EmbedFullBatch(context.Background(), texts)
	require.NoError(t, err)

	require.Len(t, sideOut, len(cpuOut))

	for i, text := range texts {
		t.Logf("--- note %d (%q) ---", i, text[:sidecarMin(50, len(text))]+"...")

		// Dense cosine similarity
		denseCos := sidecarCosine(cpuOut[i].Dense, sideOut[i].Dense)
		t.Logf("  dense cosine:  %.4f", denseCos)
		if denseCos < 0.98 {
			t.Errorf("dense cosine drift: %.4f < 0.98", denseCos)
		}

		// Sparse top-K overlap (top 10 tokens by weight)
		const topK = 10
		cpuTop := topKSparseTokens(cpuOut[i].Sparse, topK)
		sideTop := topKSparseTokens(sideOut[i].Sparse, topK)
		overlap := tokenOverlap(cpuTop, sideTop)
		t.Logf("  sparse top-%d overlap: %.2f (cpu=%d terms, side=%d terms)",
			topK, overlap, len(cpuOut[i].Sparse), len(sideOut[i].Sparse))
		if overlap < 0.7 {
			t.Errorf("sparse top-%d overlap %.2f < 0.70", topK, overlap)
		}

		// ColBERT mean per-token cosine
		colbertCos := meanColBERTCosine(cpuOut[i].ColBERT, sideOut[i].ColBERT)
		t.Logf("  colbert mean per-token cosine: %.4f (cpu=%d toks, side=%d toks)",
			colbertCos, len(cpuOut[i].ColBERT), len(sideOut[i].ColBERT))
		if colbertCos < 0.95 {
			t.Errorf("colbert cosine drift: %.4f < 0.95", colbertCos)
		}
	}
}

func sidecarCosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func topKSparseTokens(m map[int32]float32, k int) []int32 {
	type kv struct {
		k int32
		v float32
	}
	all := make([]kv, 0, len(m))
	for key, val := range m {
		all = append(all, kv{key, val})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v })
	if k > len(all) {
		k = len(all)
	}
	out := make([]int32, k)
	for i := 0; i < k; i++ {
		out[i] = all[i].k
	}
	return out
}

func tokenOverlap(a, b []int32) float64 {
	set := map[int32]bool{}
	for _, t := range a {
		set[t] = true
	}
	hits := 0
	for _, t := range b {
		if set[t] {
			hits++
		}
	}
	denom := len(a)
	if len(b) > denom {
		denom = len(b)
	}
	if denom == 0 {
		return 0
	}
	return float64(hits) / float64(denom)
}

func meanColBERTCosine(a, b [][]float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += sidecarCosine(a[i], b[i])
	}
	return sum / float64(n)
}

func sidecarMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
