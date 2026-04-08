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
		embedder, err := newDefaultEmbedder()
		if err != nil {
			return nil, nil, err
		}
		return &EmbeddingRetriever{DB: db, Embedder: embedder}, func() { _ = embedder.Close() }, nil
	case "hybrid":
		if err := requireEmbeddings(db); err != nil {
			return nil, nil, err
		}
		embedder, err := newDefaultEmbedder()
		if err != nil {
			return nil, nil, err
		}
		return &HybridRetriever{
			Retrievers: []Retriever{
				&FTSRetriever{DB: db},
				&EmbeddingRetriever{DB: db, Embedder: embedder},
			},
			K: 60,
		}, func() { _ = embedder.Close() }, nil
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
