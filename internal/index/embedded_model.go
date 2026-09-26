package index

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/embedding"
)

// EmbeddedModel names the model the vault's embeddings were made with: BGE-M3
// when any note has sparse or ColBERT vectors, MiniLM when only dense vectors
// exist, and "" for a vault that was never embedded. A write embeds its note
// with this model, so one index never mixes models.
//
//nolint:contextcheck // Open does not accept a context; these are two fast count queries
func EmbeddedModel(dbPath string) (string, error) {
	db, err := Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("opening index database: %w", err)
	}
	defer func() { _ = db.Close() }()

	var full, dense int
	if err := db.QueryRow(`SELECT
		COUNT(CASE WHEN sparse_embedding IS NOT NULL OR colbert_embedding IS NOT NULL THEN 1 END),
		COUNT(embedding) FROM notes`).Scan(&full, &dense); err != nil {
		return "", fmt.Errorf("counting embeddings: %w", err)
	}
	switch {
	case full > 0:
		return embedding.ModelBGEM3, nil
	case dense > 0:
		return embedding.ModelMiniLM, nil
	}
	return "", nil
}
