package telemetry

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Features captures aggregate, content-free statistics about a vault.
// These are the per-vault descriptors a federated telemetry pipeline
// uses to predict where population-tuned constants will and won't
// generalize (cf. the federated-paper design note H2.3).
//
// Nothing in here exposes content. Counts and a per-type distribution
// are aggregate enough to share even at the strictest privacy tier.
type Features struct {
	NoteCount        int            `json:"note_count"`
	TypeDistribution map[string]int `json:"type_distribution"`
	LinkCount        int            `json:"link_count"`
	AliasCount       int            `json:"alias_count"`
	EmbeddingCount   int            `json:"embedding_count"`
	EmbeddingDims    int            `json:"embedding_dims"`
}

// ComputeFeatures opens the vault's index DB read-only and returns the
// aggregate features. Returns an error if the index doesn't exist or
// can't be queried — callers should run `vaultmind index --vault <path>`
// before computing features for a freshly-initialized vault.
func ComputeFeatures(indexDBPath string) (*Features, error) {
	db, err := sql.Open("sqlite", "file:"+indexDBPath+"?mode=ro&immutable=1")
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}
	defer func() { _ = db.Close() }()

	f := &Features{TypeDistribution: map[string]int{}}

	if err := db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&f.NoteCount); err != nil {
		return nil, fmt.Errorf("count notes: %w", err)
	}

	rows, err := db.Query(`SELECT type, COUNT(*) FROM notes GROUP BY type`)
	if err != nil {
		return nil, fmt.Errorf("type distribution: %w", err)
	}
	for rows.Next() {
		var t sql.NullString
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan type row: %w", err)
		}
		key := t.String
		if !t.Valid || key == "" {
			key = "unstructured"
		}
		f.TypeDistribution[key] = n
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM links`).Scan(&f.LinkCount); err != nil {
		return nil, fmt.Errorf("count links: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM aliases`).Scan(&f.AliasCount); err != nil {
		return nil, fmt.Errorf("count aliases: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL`).Scan(&f.EmbeddingCount); err != nil {
		return nil, fmt.Errorf("count embeddings: %w", err)
	}
	// Embedding dims: read length of first non-null embedding blob,
	// divide by 4 (float32). Zero when no embeddings exist.
	var blobLen sql.NullInt64
	if err := db.QueryRow(`SELECT length(embedding) FROM notes WHERE embedding IS NOT NULL LIMIT 1`).Scan(&blobLen); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("embedding dim probe: %w", err)
	}
	if blobLen.Valid {
		f.EmbeddingDims = int(blobLen.Int64) / 4
	}

	return f, nil
}
