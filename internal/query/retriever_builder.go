package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/rs/zerolog/log"
)

// BuildRetriever creates the appropriate retriever for the given search mode.
// Returns a cleanup function that must be deferred if non-nil.
func BuildRetriever(mode string, db *index.DB) (retrieval.Retriever, func(), error) {
	switch mode {
	case "keyword", "":
		return &FTSRetriever{DB: db}, nil, nil
	case "semantic":
		if err := requireEmbeddings(db); err != nil {
			return nil, nil, err
		}
		embedder, cleanup, err := detectEmbedderForDB(db)
		if err != nil {
			return nil, nil, err
		}
		return &EmbeddingRetriever{DB: db, Embedder: embedder}, cleanup, nil
	case "hybrid":
		if err := requireEmbeddings(db); err != nil {
			return nil, nil, err
		}
		ret, _, cleanup, err := buildHybridRetriever(db)
		if err != nil {
			return nil, nil, err
		}
		return ret, cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unknown search mode %q (use keyword, semantic, or hybrid)", mode)
	}
}

func requireEmbeddings(db *index.DB) error {
	has, err := index.HasEmbeddings(db)
	if err != nil {
		return fmt.Errorf("checking embeddings: %w", err)
	}
	if !has {
		return fmt.Errorf("no embeddings found — run 'vaultmind index --embed' first")
	}
	return nil
}

// AutoRetrieverResult holds the retriever, embedder, and cleanup from BuildAutoRetriever.
// Retriever is always non-nil. Embedder is nil in keyword-only mode (no embeddings).
// Cleanup is always safe to call unconditionally (no-op in keyword-only mode).
type AutoRetrieverResult struct {
	Retriever retrieval.Retriever
	Embedder  embedding.Embedder // nil when keyword-only (no embeddings)
	Cleanup   func()             // always non-nil; safe to defer unconditionally
}

// BuildAutoRetriever returns a hybrid retriever if embeddings exist, otherwise keyword.
// Embedder initialization failure falls back to keyword silently.
func BuildAutoRetriever(db *index.DB) (retrieval.Retriever, func(), error) {
	r := BuildAutoRetrieverFull(db)
	return r.Retriever, r.Cleanup, nil
}

// BuildAutoRetrieverFull is like BuildAutoRetriever but also exposes the embedder
// for computing raw cosine similarities (spreading activation).
func BuildAutoRetrieverFull(db *index.DB) AutoRetrieverResult {
	noop := func() {}

	has, err := index.HasEmbeddings(db)
	if err != nil {
		log.Warn().Err(err).Msg("failed to check embeddings; falling back to keyword search")
		return AutoRetrieverResult{Retriever: &FTSRetriever{DB: db}, Cleanup: noop}
	}
	if !has {
		return AutoRetrieverResult{Retriever: &FTSRetriever{DB: db}, Cleanup: noop}
	}

	ret, embedder, cleanup, buildErr := buildHybridRetriever(db)
	if buildErr != nil {
		log.Warn().Err(buildErr).Msg("failed to initialize embedder; falling back to keyword search")
		return AutoRetrieverResult{Retriever: &FTSRetriever{DB: db}, Cleanup: noop}
	}
	return AutoRetrieverResult{Retriever: ret, Embedder: embedder, Cleanup: cleanup}
}

// buildHybridRetriever constructs the hybrid retriever with all available
// sub-retrievers and returns the embedder separately. Shared by
// BuildRetriever("hybrid") and BuildAutoRetrieverFull to avoid duplication.
func buildHybridRetriever(db *index.DB) (retrieval.Retriever, embedding.Embedder, func(), error) {
	embedder, embedderCleanup, err := detectEmbedderForDB(db)
	if err != nil {
		return nil, nil, nil, err
	}

	retrievers := []retrieval.NamedRetriever{
		{Name: "fts", Retriever: &FTSRetriever{DB: db}},
		{Name: "dense", Retriever: &EmbeddingRetriever{DB: db, Embedder: embedder}},
	}
	cleanupFuncs := []func(){embedderCleanup}

	hasSparse, sparseErr := index.HasSparseEmbeddings(db)
	if sparseErr != nil {
		// silent-failure-ok: sparse is an optional retriever lane. On
		// check failure we proceed as if not present (hasSparse stays
		// zero-value). Worst case is a retrieval without sparse signal;
		// no data is lost.
		log.Debug().Err(sparseErr).Msg("failed to check sparse embeddings")
	}
	hasColBERT, colbertErr := index.HasColBERTEmbeddings(db)
	if colbertErr != nil {
		// silent-failure-ok: same as sparse lane.
		log.Debug().Err(colbertErr).Msg("failed to check ColBERT embeddings")
	}
	if hasSparse || hasColBERT {
		if bgem3, ok := embedder.(*embedding.BGEM3Embedder); ok {
			if hasSparse {
				retrievers = append(retrievers, retrieval.NamedRetriever{Name: "sparse", Retriever: &SparseRetriever{DB: db, EmbedSparse: bgem3.EmbedSparse}})
			}
			if hasColBERT {
				retrievers = append(retrievers, retrieval.NamedRetriever{Name: "colbert", Retriever: &ColBERTRetriever{DB: db, EmbedColBERT: bgem3.EmbedColBERT, Dims: embedding.BGEM3Dims}})
			}
		}
	}

	cleanup := func() {
		for _, f := range cleanupFuncs {
			f()
		}
	}
	return &HybridRetriever{Retrievers: retrievers, K: DefaultRRFK}, embedder, cleanup, nil
}

// BuildAutoRetrieverWithActivation is BuildAutoRetrieverFull plus the
// activation lane (slice 5b' from reference-plasticity-priority-order).
// When expDB is non-nil and the underlying retriever is hybrid, an
// ActivationRetriever is appended as a 5th RRF lane named "activation".
// Notes with access_count = 0 simply don't appear in this lane, so
// cold-start vaults degrade cleanly to 4-way RRF.
//
// IMPORTANT calibration obligation: enabling activation shifts the
// rank-1/rank-2 score gap distribution that step-4's TopHitConfidence
// thresholds (5% / 2%) were calibrated against. Callers turning this
// on for the default ask path must re-probe the gap distribution and
// update the threshold constants in internal/query/format.go, or the
// strong/moderate/weak labels silently miscalibrate. See the priority-
// order doc for the full step-4 ↔ step-5 coupling.
func BuildAutoRetrieverWithActivation(db *index.DB, expDB *experiment.DB) AutoRetrieverResult {
	res := BuildAutoRetrieverFull(db)
	if expDB == nil {
		return res
	}
	hr, ok := res.Retriever.(*HybridRetriever)
	if !ok {
		// 4-way isn't available (no embeddings) — keyword-only fallback,
		// no activation lane to add. Same shape as the current contract.
		return res
	}
	hr.Retrievers = append(hr.Retrievers, retrieval.NamedRetriever{
		Name: "activation",
		Retriever: &ActivationRetriever{
			DB:     db,
			ExpDB:  expDB,
			Params: experiment.DefaultActivationParams(0.5),
		},
	})
	return res
}

func newDefaultEmbedder() (*embedding.HugotEmbedder, error) {
	embedder, err := embedding.NewHugotEmbedder(embedding.DefaultHugotConfig())
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}
	return embedder, nil
}

// detectEmbedderForDB inspects stored embedding dimensions to determine the
// correct embedder. Uses the per-dims breakdown so mixed-state vaults
// (mid-migration from MiniLM to BGE-M3) load BGE-M3 — picking BGE-M3 when
// any rows are BGE-M3 is strictly safer than the inverse. BGE-M3 queries
// can match against BGE-M3 rows via all four lanes (dense + sparse +
// colbert + fts); the minority MiniLM rows are temporarily un-matchable on
// dense (dim mismatch, 384 vs 1024) until they're re-embedded. Loading
// MiniLM on a mostly-BGE-M3 vault would lose sparse + colbert entirely AND
// fail to match the BGE-M3 majority's dense rows. See vaultmind#32.
func detectEmbedderForDB(db *index.DB) (embedding.Embedder, func(), error) {
	counts, err := index.DetectEmbeddingDimsCounts(db)
	if err != nil {
		// silent-failure-ok: dims detection falls back to MiniLM default,
		// which is the correct choice for a vault that hasn't been embedded
		// at all. The embedder init boundary fails loudly if the model
		// can't load.
		log.Debug().Err(err).Msg("failed to count embedding dims, falling back to MiniLM")
		return newMiniLMEmbedder()
	}
	hasBGEM3 := false
	hasMiniLM := false
	for _, c := range counts {
		switch c.Dims {
		case embedding.BGEM3Dims:
			hasBGEM3 = true
		case 384:
			hasMiniLM = true
		}
	}
	if hasBGEM3 && hasMiniLM {
		log.Warn().Msg("vault is in mixed-model state (MiniLM + BGE-M3); loading BGE-M3 — run 'vaultmind index --embed --model bge-m3' to converge")
	}
	if hasBGEM3 {
		bgem3, bgem3Err := embedding.NewBGEM3Embedder(embedding.BGEM3Config())
		if bgem3Err != nil {
			return nil, nil, fmt.Errorf("creating BGE-M3 embedder: %w", bgem3Err)
		}
		return bgem3, func() { _ = bgem3.Close() }, nil
	}
	return newMiniLMEmbedder()
}

// newMiniLMEmbedder constructs the default MiniLM embedder with a uniform
// cleanup signature. Extracted to keep the mixed-model branching in
// detectEmbedderForDB readable.
func newMiniLMEmbedder() (embedding.Embedder, func(), error) {
	embedder, err := newDefaultEmbedder()
	if err != nil {
		return nil, nil, err
	}
	return embedder, func() { _ = embedder.Close() }, nil
}
