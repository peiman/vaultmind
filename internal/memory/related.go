package memory

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
)

// RelatedConfig holds parameters for a Related operation.
type RelatedConfig struct {
	Input string
	Mode  string // "mixed", "explicit", or "inferred"
}

// RelatedItem is a single related note returned by Related.
type RelatedItem struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	EdgeType   string  `json:"edge_type"`
	Confidence string  `json:"confidence"`
	Origin     string  `json:"origin"`
	Score      float64 `json:"score,omitempty"`
}

// RelatedResult is the full output of a Related operation.
type RelatedResult struct {
	TargetID string        `json:"target_id"`
	Mode     string        `json:"mode"`
	Related  []RelatedItem `json:"related"`
}

// relatedEdge is an internal helper holding raw edge data before enrichment.
type relatedEdge struct {
	noteID     string
	edgeType   string
	confidence string
	origin     string
	weight     float64
}

// Related resolves the input entity and returns directly connected notes,
// filtered by the given mode ("mixed", "explicit", or "inferred").
func Related(resolver *graph.Resolver, db *index.DB, cfg RelatedConfig) (*RelatedResult, error) {
	// Step 1: Resolve the input to a canonical note ID.
	resolved, err := resolver.Resolve(cfg.Input)
	if err != nil {
		return nil, fmt.Errorf("resolving input %q: %w", cfg.Input, err)
	}
	if !resolved.Resolved || len(resolved.Matches) == 0 {
		return nil, fmt.Errorf("could not resolve %q to a known note", cfg.Input)
	}
	targetID := resolved.Matches[0].ID

	// Step 2: Query outbound edges (src_note_id = target).
	var edges []relatedEdge

	outRows, err := db.Query(
		`SELECT dst_note_id, edge_type, confidence, origin, COALESCE(weight, 0)
		 FROM links
		 WHERE src_note_id = ? AND resolved = TRUE AND dst_note_id IS NOT NULL`,
		targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying outbound edges: %w", err)
	}
	defer func() { _ = outRows.Close() }()

	for outRows.Next() {
		var e relatedEdge
		if scanErr := outRows.Scan(&e.noteID, &e.edgeType, &e.confidence, &e.origin, &e.weight); scanErr != nil {
			return nil, fmt.Errorf("scanning outbound edge: %w", scanErr)
		}
		edges = append(edges, e)
	}
	if err := outRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating outbound edges: %w", err)
	}

	// Step 3: Query inbound edges (dst_note_id = target).
	inRows, err := db.Query(
		`SELECT src_note_id, edge_type, confidence, origin, COALESCE(weight, 0)
		 FROM links
		 WHERE dst_note_id = ? AND resolved = TRUE`,
		targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying inbound edges: %w", err)
	}
	defer func() { _ = inRows.Close() }()

	for inRows.Next() {
		var e relatedEdge
		if scanErr := inRows.Scan(&e.noteID, &e.edgeType, &e.confidence, &e.origin, &e.weight); scanErr != nil {
			return nil, fmt.Errorf("scanning inbound edge: %w", scanErr)
		}
		edges = append(edges, e)
	}
	if err := inRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inbound edges: %w", err)
	}

	// Step 4: Filter by mode and deduplicate by note ID (keep first occurrence).
	seen := make(map[string]bool)
	result := &RelatedResult{
		TargetID: targetID,
		Mode:     cfg.Mode,
		Related:  []RelatedItem{},
	}

	for _, e := range edges {
		if seen[e.noteID] {
			continue
		}

		keep := false
		switch cfg.Mode {
		case "explicit":
			keep = e.confidence == "high"
		case "inferred":
			keep = e.confidence == "medium" || e.confidence == "low"
		default: // "mixed"
			keep = true
		}

		if !keep {
			continue
		}
		seen[e.noteID] = true

		// Step 5: Load basic metadata for this note.
		row, err := db.QueryNoteByID(e.noteID)
		if err != nil {
			return nil, fmt.Errorf("querying note %q: %w", e.noteID, err)
		}

		item := RelatedItem{
			ID:         e.noteID,
			EdgeType:   e.edgeType,
			Confidence: e.confidence,
			Origin:     e.origin,
		}

		if row != nil {
			item.Type = row.Type
			item.Title = row.Title
		}

		// Step 6: For tag_overlap edges, include weight as Score.
		if e.edgeType == "tag_overlap" {
			item.Score = e.weight
		}

		result.Related = append(result.Related, item)
	}

	return result, nil
}
