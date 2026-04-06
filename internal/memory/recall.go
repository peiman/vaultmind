package memory

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
)

// RecallConfig holds parameters for a Recall operation.
type RecallConfig struct {
	Input         string
	Depth         int
	MinConfidence string
	MaxNodes      int
}

// RecallNode is an enriched graph node returned by Recall.
type RecallNode struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Title          string                 `json:"title"`
	Distance       int                    `json:"distance"`
	EdgeFromParent *graph.TraverseEdge    `json:"edge_from_parent,omitempty"`
	Frontmatter    map[string]interface{} `json:"frontmatter"`
}

// RecallEdge describes a directed edge between two nodes in the result.
type RecallEdge struct {
	SourceID   string `json:"source_id"`
	TargetID   string `json:"target_id"`
	EdgeType   string `json:"edge_type"`
	Confidence string `json:"confidence"`
}

// RecallResult is the full output of a Recall operation.
type RecallResult struct {
	TargetID        string       `json:"target_id"`
	Depth           int          `json:"depth"`
	MaxNodes        int          `json:"max_nodes"`
	MaxNodesReached bool         `json:"max_nodes_reached"`
	Nodes           []RecallNode `json:"nodes"`
	Edges           []RecallEdge `json:"edges"`
}

// Recall resolves the input entity and returns an enriched subgraph neighbourhood.
// It resolves the input via the resolver, traverses the graph using BFS, and
// enriches each traversal node with type, title, and frontmatter from the database.
func Recall(resolver *graph.Resolver, db *index.DB, cfg RecallConfig) (*RecallResult, error) {
	// Step 1: Resolve the input to a canonical note ID.
	resolved, err := resolver.Resolve(cfg.Input)
	if err != nil {
		return nil, fmt.Errorf("resolving input %q: %w", cfg.Input, err)
	}
	if !resolved.Resolved || len(resolved.Matches) == 0 {
		return nil, fmt.Errorf("could not resolve %q to a known note", cfg.Input)
	}
	targetID := resolved.Matches[0].ID

	// Step 2: Traverse the graph from the resolved node.
	traversal, err := resolver.Traverse(graph.TraverseConfig{
		StartID:       targetID,
		MaxDepth:      cfg.Depth,
		MinConfidence: cfg.MinConfidence,
		MaxNodes:      cfg.MaxNodes,
	})
	if err != nil {
		return nil, fmt.Errorf("traversing graph from %q: %w", targetID, err)
	}

	// Step 3: Build the result, enriching each node with database info.
	result := &RecallResult{
		TargetID:        targetID,
		Depth:           cfg.Depth,
		MaxNodes:        cfg.MaxNodes,
		MaxNodesReached: traversal.MaxNodesReached,
		Nodes:           []RecallNode{},
		Edges:           []RecallEdge{},
	}

	for _, tn := range traversal.Nodes {
		full, err := db.QueryFullNote(tn.ID)
		if err != nil {
			return nil, fmt.Errorf("querying full note %q: %w", tn.ID, err)
		}

		node := RecallNode{
			ID:             tn.ID,
			Distance:       tn.Distance,
			EdgeFromParent: tn.EdgeFrom,
		}

		if full != nil {
			node.Type = full.Type
			node.Title = full.Title
			node.Frontmatter = full.Frontmatter
		} else {
			node.Frontmatter = map[string]interface{}{}
		}

		result.Nodes = append(result.Nodes, node)

		// Build an edge entry for each non-root node (nodes with an edge from a parent).
		if tn.EdgeFrom != nil {
			result.Edges = append(result.Edges, RecallEdge{
				SourceID:   tn.EdgeFrom.SourceID,
				TargetID:   tn.ID,
				EdgeType:   tn.EdgeFrom.EdgeType,
				Confidence: tn.EdgeFrom.Confidence,
			})
		}
	}

	return result, nil
}
