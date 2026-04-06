package query

import "github.com/peiman/vaultmind/internal/graph"

// NeighborsResult holds the result of a neighbors traversal query.
type NeighborsResult struct {
	StartID         string         `json:"start_id"`
	Nodes           []NeighborNode `json:"nodes"`
	MaxNodesReached bool           `json:"max_nodes_reached"`
}

// NeighborNode represents a single node in the neighbors result.
type NeighborNode struct {
	ID       string        `json:"id"`
	Distance int           `json:"distance"`
	EdgeFrom *NeighborEdge `json:"edge_from,omitempty"`
}

// NeighborEdge describes the edge by which a neighbor was reached.
type NeighborEdge struct {
	SourceID   string  `json:"source_id"`
	EdgeType   string  `json:"edge_type"`
	Confidence string  `json:"confidence"`
	Weight     float64 `json:"weight"`
}

// Neighbors performs a BFS traversal from the given input (resolved via
// the Resolver's entity resolution) and returns the result in a format
// suitable for JSON output.
func Neighbors(resolver *graph.Resolver, input string, depth int, minConfidence string, maxNodes int) (*NeighborsResult, error) {
	resolveResult, err := resolver.Resolve(input)
	if err != nil {
		return nil, err
	}
	startID := input
	if resolveResult.Resolved && len(resolveResult.Matches) > 0 {
		startID = resolveResult.Matches[0].ID
	}

	tr, err := resolver.Traverse(graph.TraverseConfig{
		StartID:       startID,
		MaxDepth:      depth,
		MinConfidence: minConfidence,
		MaxNodes:      maxNodes,
	})
	if err != nil {
		return nil, err
	}

	result := &NeighborsResult{
		StartID:         tr.StartID,
		MaxNodesReached: tr.MaxNodesReached,
		Nodes:           make([]NeighborNode, len(tr.Nodes)),
	}
	for i, tn := range tr.Nodes {
		node := NeighborNode{ID: tn.ID, Distance: tn.Distance}
		if tn.EdgeFrom != nil {
			node.EdgeFrom = &NeighborEdge{
				SourceID:   tn.EdgeFrom.SourceID,
				EdgeType:   tn.EdgeFrom.EdgeType,
				Confidence: tn.EdgeFrom.Confidence,
				Weight:     tn.EdgeFrom.Weight,
			}
		}
		result.Nodes[i] = node
	}
	return result, nil
}
