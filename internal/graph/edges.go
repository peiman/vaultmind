package graph

import (
	"database/sql"
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
)

// LinkResult represents a single edge in link query results.
type LinkResult struct {
	TargetID    *string `json:"target_id"`
	TargetTitle string  `json:"target_title,omitempty"`
	TargetPath  string  `json:"target_path,omitempty"`
	TargetRaw   string  `json:"target_raw,omitempty"`
	SourceID    string  `json:"source_id,omitempty"`
	SourceTitle string  `json:"source_title,omitempty"`
	SourcePath  string  `json:"source_path,omitempty"`
	EdgeType    string  `json:"edge_type"`
	Confidence  string  `json:"confidence"`
	Origin      string  `json:"origin,omitempty"`
	Resolved    bool    `json:"resolved"`
}

// LinksOut returns all outbound edges from the given note.
// If edgeTypeFilter is non-empty, only edges of that type are returned.
func LinksOut(db *index.DB, noteID, edgeTypeFilter string) ([]LinkResult, error) {
	q := `SELECT l.dst_note_id, n.title, n.path, l.dst_raw, l.edge_type,
		l.confidence, l.origin, l.resolved
		FROM links l
		LEFT JOIN notes n ON n.id = l.dst_note_id
		WHERE l.src_note_id = ?`
	args := []interface{}{noteID}

	if edgeTypeFilter != "" {
		q += " AND l.edge_type = ?"
		args = append(args, edgeTypeFilter)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying outbound links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []LinkResult
	for rows.Next() {
		var r LinkResult
		var dstID, title, path, origin sql.NullString
		if scanErr := rows.Scan(&dstID, &title, &path, &r.TargetRaw, &r.EdgeType,
			&r.Confidence, &origin, &r.Resolved); scanErr != nil {
			return nil, fmt.Errorf("scanning link: %w", scanErr)
		}
		if dstID.Valid {
			r.TargetID = &dstID.String
		}
		r.TargetTitle = title.String
		r.TargetPath = path.String
		r.Origin = origin.String
		results = append(results, r)
	}
	return results, rows.Err()
}

// LinksIn returns all inbound edges pointing to the given note.
// Uses dst_raw matching for unresolved edges and dst_note_id for resolved ones.
func LinksIn(db *index.DB, noteID, edgeTypeFilter string) ([]LinkResult, error) {
	// Match on dst_note_id (resolved) OR dst_raw (for explicit_relation edges
	// where dst_raw stores the target note's stable ID before resolution).
	q := `SELECT l.src_note_id, n.title, n.path, l.dst_raw, l.edge_type,
		l.confidence, l.origin, l.resolved
		FROM links l
		JOIN notes n ON n.id = l.src_note_id
		WHERE (l.dst_note_id = ? OR l.dst_raw = ?)`
	args := []interface{}{noteID, noteID}

	if edgeTypeFilter != "" {
		q += " AND l.edge_type = ?"
		args = append(args, edgeTypeFilter)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying inbound links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []LinkResult
	for rows.Next() {
		var r LinkResult
		var origin sql.NullString
		if scanErr := rows.Scan(&r.SourceID, &r.SourceTitle, &r.SourcePath, &r.TargetRaw,
			&r.EdgeType, &r.Confidence, &origin, &r.Resolved); scanErr != nil {
			return nil, fmt.Errorf("scanning inbound link: %w", scanErr)
		}
		r.Origin = origin.String
		results = append(results, r)
	}
	return results, rows.Err()
}
