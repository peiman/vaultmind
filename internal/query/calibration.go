package query

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
)

// maxNTNPairs caps how many note-to-note cosine pairs are summed for the
// dispersion stats. Below it, all pairs are used; above it, pairs are strided
// deterministically (no RNG) so the measurement is reproducible and bounded
// (a 10k-note vault has ~50M pairs — far too many to sum every time).
const maxNTNPairs = 200_000

// MeasuredCalibration is the raw per-vault calibration measured from the
// embedding space. The cmd layer stamps it with an id + timestamp and stores
// it as an experiment.CalibrationSnapshot.
type MeasuredCalibration struct {
	NoiseFloor       float64
	NoiseFloorProbes int
	ProbeSetVersion  int
	NTNCosineMu      float64
	NTNCosineSigma   float64
	NTNSampleCount   int
	NoteCount        int
	EmbeddingDims    int
}

// MeasureNoiseFloor measures a vault's noise floor N and note-to-note cosine
// dispersion from its stored dense embeddings. N is the max cosine any of the
// fixed off-topic probe strings reaches against any note — the cosine an
// off-domain query gets, which the relevance metric R = top_cosine - N is
// measured against. Requires an embedder (for the probes) and embedded notes.
func MeasureNoiseFloor(ctx context.Context, embedder embedding.Embedder, db *index.DB) (*MeasuredCalibration, error) {
	if embedder == nil {
		return nil, fmt.Errorf("noise-floor measurement requires an embedder")
	}
	all, err := index.LoadAllEmbeddings(db)
	if err != nil {
		return nil, fmt.Errorf("loading embeddings for calibration: %w", err)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no embeddings to calibrate — run 'vaultmind index --embed' first")
	}

	// Mixed-model guard: refuse to calibrate a vault whose embeddings don't all
	// match the active embedder's dimensionality. CosineSimilarity returns 0 on
	// a dim mismatch, which would silently drag N and the dispersion toward
	// garbage and store a corrupt snapshot as federated evidence. A mixed vault
	// is a transient mid-migration state — re-embed to converge first.
	qDims := embedder.Dims()
	for _, ne := range all {
		if len(ne.Embedding) != qDims {
			return nil, fmt.Errorf(
				"calibration aborted: vault has mixed-dimensionality embeddings (found %d, embedder is %d) — re-embed to a single model before calibrating",
				len(ne.Embedding), qDims)
		}
	}
	// Sort by note id so the strided note-to-note sampling (and thus the stored
	// mu/sigma) is reproducible across runs — SQLite does not guarantee row
	// order without ORDER BY.
	sort.Slice(all, func(i, j int) bool { return all[i].NoteID < all[j].NoteID })

	// N = max over probes of (max cosine to any note).
	var n float64
	for _, probe := range noisefloor.DefaultProbes {
		qv, embErr := embedder.Embed(ctx, probe)
		if embErr != nil {
			return nil, fmt.Errorf("embedding calibration probe: %w", embErr)
		}
		for _, ne := range all {
			if c := CosineSimilarity(qv, ne.Embedding); c > n {
				n = c
			}
		}
	}

	mu, sigma, count := noteToNoteStats(all)
	return &MeasuredCalibration{
		NoiseFloor:       n,
		NoiseFloorProbes: len(noisefloor.DefaultProbes),
		ProbeSetVersion:  noisefloor.ProbeSetVersion,
		NTNCosineMu:      mu,
		NTNCosineSigma:   sigma,
		NTNSampleCount:   count,
		NoteCount:        len(all),
		EmbeddingDims:    qDims, // guaranteed uniform by the mixed-model guard above
	}, nil
}

// noteToNoteStats computes the mean and population standard deviation of the
// pairwise cosine similarity between stored note embeddings — the vault's
// embedding-space dispersion. Strides deterministically over pairs when there
// are more than maxNTNPairs. Returns (0,0,0) for fewer than two notes.
func noteToNoteStats(all []index.NoteEmbedding) (mu, sigma float64, count int) {
	if len(all) < 2 {
		return 0, 0, 0
	}
	totalPairs := len(all) * (len(all) - 1) / 2
	stride := 1
	if totalPairs > maxNTNPairs {
		// Ceil division so the sampled count stays at/under maxNTNPairs (a hard
		// bound), not up to ~2x as floor division would allow.
		stride = (totalPairs + maxNTNPairs - 1) / maxNTNPairs
	}

	var sum float64
	var sims []float64
	idx := 0
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if idx%stride == 0 {
				c := CosineSimilarity(all[i].Embedding, all[j].Embedding)
				sims = append(sims, c)
				sum += c
			}
			idx++
		}
	}
	count = len(sims)
	if count == 0 {
		return 0, 0, 0
	}
	mu = sum / float64(count)
	var variance float64
	for _, c := range sims {
		variance += (c - mu) * (c - mu)
	}
	sigma = math.Sqrt(variance / float64(count))
	return mu, sigma, count
}
