package graph

import (
	"fmt"
)

// ConfidenceLevel maps confidence label to numeric rank (higher = more confident).
var ConfidenceLevel = map[string]int{
	"low":    1,
	"medium": 2,
	"high":   3,
}

// MeetsConfidence reports whether edgeConf satisfies the minConf threshold.
// An edge meets confidence if its level is >= the minimum required level.
func MeetsConfidence(edgeConf, minConf string) bool {
	edge := ConfidenceLevel[edgeConf]
	minLevel := ConfidenceLevel[minConf]
	if edge == 0 || minLevel == 0 {
		return false
	}
	return edge >= minLevel
}

// TraverseConfig holds parameters for a BFS traversal.
type TraverseConfig struct {
	StartID       string
	MaxDepth      int
	MinConfidence string
	MaxNodes      int
}

// TraverseEdge describes the edge by which a node was reached.
type TraverseEdge struct {
	SourceID   string  `json:"source_id"`
	EdgeType   string  `json:"edge_type"`
	Confidence string  `json:"confidence"`
	Weight     float64 `json:"weight"`
}

// TraverseNode represents a node in the BFS result.
type TraverseNode struct {
	ID       string        `json:"id"`
	Distance int           `json:"distance"`
	EdgeFrom *TraverseEdge `json:"edge_from,omitempty"`
}

// TraverseResult is the full output of a BFS traversal.
type TraverseResult struct {
	StartID         string         `json:"start_id"`
	Nodes           []TraverseNode `json:"nodes"`
	MaxNodesReached bool           `json:"max_nodes_reached"`
}

// neighbor is an internal type for adjacency results.
type neighbor struct {
	id         string
	edgeType   string
	confidence string
	weight     float64
	sourceID   string // the node we came from
}

// Traverse performs a BFS from cfg.StartID up to cfg.MaxDepth hops,
// respecting cfg.MinConfidence and cfg.MaxNodes limits.
// Both outbound and inbound resolved edges are followed (bidirectional).
func (r *Resolver) Traverse(cfg TraverseConfig) (*TraverseResult, error) {
	result := &TraverseResult{StartID: cfg.StartID}

	visited := make(map[string]bool)
	visited[cfg.StartID] = true

	startNode := TraverseNode{ID: cfg.StartID, Distance: 0}
	result.Nodes = append(result.Nodes, startNode)

	if cfg.MaxNodes > 0 && len(result.Nodes) >= cfg.MaxNodes {
		return result, nil
	}

	// BFS queue: each entry is (nodeID, depth)
	type queueItem struct {
		id    string
		depth int
	}
	queue := []queueItem{{id: cfg.StartID, depth: 0}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= cfg.MaxDepth {
			continue
		}

		neighbors, err := r.queryNeighbors(item.id, cfg.MinConfidence)
		if err != nil {
			return nil, fmt.Errorf("querying neighbors of %s: %w", item.id, err)
		}

		for _, nb := range neighbors {
			if visited[nb.id] {
				continue
			}
			visited[nb.id] = true

			node := TraverseNode{
				ID:       nb.id,
				Distance: item.depth + 1,
				EdgeFrom: &TraverseEdge{
					SourceID:   nb.sourceID,
					EdgeType:   nb.edgeType,
					Confidence: nb.confidence,
					Weight:     nb.weight,
				},
			}
			result.Nodes = append(result.Nodes, node)

			if cfg.MaxNodes > 0 && len(result.Nodes) >= cfg.MaxNodes {
				result.MaxNodesReached = true
				return result, nil
			}

			queue = append(queue, queueItem{id: nb.id, depth: item.depth + 1})
		}
	}

	return result, nil
}

// queryNeighbors returns all neighbors (outbound + inbound) of nodeID
// where edges are resolved and meet the minConfidence threshold.
func (r *Resolver) queryNeighbors(nodeID, minConfidence string) ([]neighbor, error) {
	var results []neighbor

	// Outbound: edges where nodeID is the source
	outRows, err := r.db.Query(
		`SELECT dst_note_id, edge_type, confidence, COALESCE(weight, 0)
		 FROM links
		 WHERE src_note_id = ? AND resolved = TRUE AND dst_note_id IS NOT NULL`,
		nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("outbound query: %w", err)
	}
	defer func() { _ = outRows.Close() }()

	for outRows.Next() {
		var nb neighbor
		nb.sourceID = nodeID
		if err := outRows.Scan(&nb.id, &nb.edgeType, &nb.confidence, &nb.weight); err != nil {
			return nil, fmt.Errorf("scanning outbound row: %w", err)
		}
		if MeetsConfidence(nb.confidence, minConfidence) {
			results = append(results, nb)
		}
	}
	if err := outRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating outbound rows: %w", err)
	}

	// Inbound: edges where nodeID is the destination
	inRows, err := r.db.Query(
		`SELECT src_note_id, edge_type, confidence, COALESCE(weight, 0)
		 FROM links
		 WHERE dst_note_id = ? AND resolved = TRUE`,
		nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("inbound query: %w", err)
	}
	defer func() { _ = inRows.Close() }()

	for inRows.Next() {
		var nb neighbor
		nb.sourceID = nodeID
		if err := inRows.Scan(&nb.id, &nb.edgeType, &nb.confidence, &nb.weight); err != nil {
			return nil, fmt.Errorf("scanning inbound row: %w", err)
		}
		if MeetsConfidence(nb.confidence, minConfidence) {
			results = append(results, nb)
		}
	}
	if err := inRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inbound rows: %w", err)
	}

	return results, nil
}
