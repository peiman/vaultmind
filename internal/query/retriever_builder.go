package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
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

		// Auto-detect BGE-M3 columns for 4-way hybrid
		hasSparse, _ := index.HasSparseEmbeddings(db)
		hasColBERT, _ := index.HasColBERTEmbeddings(db)
		if hasSparse || hasColBERT {
			bgem3, bgem3Err := embedding.NewBGEM3Embedder(embedding.BGEM3Config())
			if bgem3Err == nil {
				cleanupFuncs = append(cleanupFuncs, func() { _ = bgem3.Close() })
				if hasSparse {
					retrievers = append(retrievers, &SparseRetriever{DB: db, EmbedSparse: bgem3.EmbedSparse})
				}
				if hasColBERT {
					retrievers = append(retrievers, &ColBERTRetriever{DB: db, EmbedColBERT: bgem3.EmbedColBERT, Dims: embedding.BGEM3Dims})
				}
			}
			// If BGE-M3 embedder fails, fall back to 2-way hybrid
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

// BuildAutoRetriever returns a hybrid retriever if embeddings exist, otherwise keyword.
// Embedder initialization failure falls back to keyword silently.
func BuildAutoRetriever(db *index.DB) (Retriever, func(), error) {
	has, err := index.HasEmbeddings(db)
	if err != nil || !has {
		return &FTSRetriever{DB: db}, nil, nil //nolint:nilerr // intentional fallback to keyword
	}
	ret, cleanup, err := BuildRetriever("hybrid", db)
	if err != nil {
		return &FTSRetriever{DB: db}, nil, nil //nolint:nilerr // intentional fallback to keyword
	}
	return ret, cleanup, nil
}

func newDefaultEmbedder() (*embedding.HugotEmbedder, error) {
	embedder, err := embedding.NewHugotEmbedder(embedding.DefaultHugotConfig())
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}
	return embedder, nil
}

// detectEmbedderForDB inspects stored embeddings to determine the correct embedder.
// If embeddings are 1024-dim (BGE-M3), creates a BGE-M3 embedder; otherwise MiniLM.
// Returns the embedder and a cleanup function.
func detectEmbedderForDB(db *index.DB) (embedding.Embedder, func(), error) {
	all, err := index.LoadAllEmbeddings(db)
	if err == nil && len(all) > 0 {
		dims := len(all[0].Embedding)
		if dims == embedding.BGEM3Dims {
			bgem3, bgem3Err := embedding.NewBGEM3Embedder(embedding.BGEM3Config())
			if bgem3Err != nil {
				return nil, nil, fmt.Errorf("creating BGE-M3 embedder: %w", bgem3Err)
			}
			return bgem3, func() { _ = bgem3.Close() }, nil
		}
	}
	embedder, err := newDefaultEmbedder()
	if err != nil {
		return nil, nil, err
	}
	return embedder, func() { _ = embedder.Close() }, nil
}
