package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/rs/zerolog/log"
)

// BuildRetriever creates the appropriate retriever for the given search mode.
// Returns a cleanup function that must be deferred if non-nil.
func BuildRetriever(mode string, db *index.DB) (Retriever, func(), error) {
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
		embedder, embedderCleanup, err := detectEmbedderForDB(db)
		if err != nil {
			return nil, nil, err
		}
		retrievers := []Retriever{
			&FTSRetriever{DB: db},
			&EmbeddingRetriever{DB: db, Embedder: embedder},
		}
		cleanupFuncs := []func(){embedderCleanup}

		// Auto-detect BGE-M3 columns for 4-way hybrid.
		// Reuse the existing embedder if it's already a BGEM3Embedder (C1 fix).
		hasSparse, sparseErr := index.HasSparseEmbeddings(db)
		if sparseErr != nil {
			log.Debug().Err(sparseErr).Msg("failed to check sparse embeddings")
		}
		hasColBERT, colbertErr := index.HasColBERTEmbeddings(db)
		if colbertErr != nil {
			log.Debug().Err(colbertErr).Msg("failed to check ColBERT embeddings")
		}
		if hasSparse || hasColBERT {
			if bgem3, ok := embedder.(*embedding.BGEM3Embedder); ok {
				// Reuse the already-loaded BGE-M3 embedder — no double init
				if hasSparse {
					retrievers = append(retrievers, &SparseRetriever{DB: db, EmbedSparse: bgem3.EmbedSparse})
				}
				if hasColBERT {
					retrievers = append(retrievers, &ColBERTRetriever{DB: db, EmbedColBERT: bgem3.EmbedColBERT, Dims: embedding.BGEM3Dims})
				}
			}
			// If embedder is MiniLM but sparse/ColBERT columns exist, ignore them
			// (would need BGE-M3 to query-embed sparse/ColBERT)
		}

		cleanup := func() {
			for _, f := range cleanupFuncs {
				f()
			}
		}
		return &HybridRetriever{Retrievers: retrievers, K: 60}, cleanup, nil
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
type AutoRetrieverResult struct {
	Retriever Retriever
	Embedder  embedding.Embedder // nil when keyword-only (no embeddings)
	Cleanup   func()
}

// BuildAutoRetriever returns a hybrid retriever if embeddings exist, otherwise keyword.
// Embedder initialization failure falls back to keyword silently.
// The returned Embedder can be used for computing raw cosine similarities
// (e.g., for spreading activation) without loading the model twice.
func BuildAutoRetriever(db *index.DB) (Retriever, func(), error) {
	r := BuildAutoRetrieverFull(db)
	return r.Retriever, r.Cleanup, nil
}

// BuildAutoRetrieverFull is like BuildAutoRetriever but also exposes the embedder
// for computing raw cosine similarities (spreading activation).
func BuildAutoRetrieverFull(db *index.DB) AutoRetrieverResult {
	has, err := index.HasEmbeddings(db)
	if err != nil || !has {
		return AutoRetrieverResult{Retriever: &FTSRetriever{DB: db}}
	}

	embedder, embedderCleanup, err := detectEmbedderForDB(db)
	if err != nil {
		return AutoRetrieverResult{Retriever: &FTSRetriever{DB: db}}
	}

	retrievers := []Retriever{
		&FTSRetriever{DB: db},
		&EmbeddingRetriever{DB: db, Embedder: embedder},
	}
	cleanupFuncs := []func(){embedderCleanup}

	// Auto-detect BGE-M3 columns for 4-way hybrid.
	hasSparse, sparseErr := index.HasSparseEmbeddings(db)
	if sparseErr != nil {
		log.Debug().Err(sparseErr).Msg("failed to check sparse embeddings")
	}
	hasColBERT, colbertErr := index.HasColBERTEmbeddings(db)
	if colbertErr != nil {
		log.Debug().Err(colbertErr).Msg("failed to check ColBERT embeddings")
	}
	if hasSparse || hasColBERT {
		if bgem3, ok := embedder.(*embedding.BGEM3Embedder); ok {
			if hasSparse {
				retrievers = append(retrievers, &SparseRetriever{DB: db, EmbedSparse: bgem3.EmbedSparse})
			}
			if hasColBERT {
				retrievers = append(retrievers, &ColBERTRetriever{DB: db, EmbedColBERT: bgem3.EmbedColBERT, Dims: embedding.BGEM3Dims})
			}
		}
	}

	cleanup := func() {
		for _, f := range cleanupFuncs {
			f()
		}
	}
	return AutoRetrieverResult{
		Retriever: &HybridRetriever{Retrievers: retrievers, K: 60},
		Embedder:  embedder,
		Cleanup:   cleanup,
	}
}

func newDefaultEmbedder() (*embedding.HugotEmbedder, error) {
	embedder, err := embedding.NewHugotEmbedder(embedding.DefaultHugotConfig())
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}
	return embedder, nil
}

// detectEmbedderForDB inspects stored embedding dimensions to determine the correct embedder.
// Uses a single-row LENGTH query instead of loading all embeddings (C3 fix).
func detectEmbedderForDB(db *index.DB) (embedding.Embedder, func(), error) {
	dims, err := index.DetectEmbeddingDims(db)
	if err != nil {
		log.Debug().Err(err).Msg("failed to detect embedding dims, falling back to MiniLM")
	}
	if err == nil && dims == embedding.BGEM3Dims {
		bgem3, bgem3Err := embedding.NewBGEM3Embedder(embedding.BGEM3Config())
		if bgem3Err != nil {
			return nil, nil, fmt.Errorf("creating BGE-M3 embedder: %w", bgem3Err)
		}
		return bgem3, func() { _ = bgem3.Close() }, nil
	}
	embedder, defaultErr := newDefaultEmbedder()
	if defaultErr != nil {
		return nil, nil, defaultErr
	}
	return embedder, func() { _ = embedder.Close() }, nil
}
