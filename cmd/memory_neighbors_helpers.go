package cmd

import (
	"fmt"
	"io"

	memory "github.com/peiman/vaultmind/internal/memory"
)

// formatRecall renders the human-readable neighbors traversal: the target at
// depth 0, then each neighbor with its distance. Shared by memory neighbors
// and its deprecated alias memory recall.
func formatRecall(result *memory.RecallResult, w io.Writer) error {
	for _, n := range result.Nodes {
		if n.Distance == 0 {
			if _, err := fmt.Fprintf(w, "%s [%s] %q (depth 0)\n", n.ID, n.Type, n.Title); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "  → %s [%s] %q depth %d\n", n.ID, n.Type, n.Title, n.Distance); err != nil {
				return err
			}
		}
	}
	suffix := ""
	if result.MaxNodesReached {
		suffix = " (max reached)"
	}
	_, err := fmt.Fprintf(w, "%d nodes%s\n", len(result.Nodes), suffix)
	return err
}
