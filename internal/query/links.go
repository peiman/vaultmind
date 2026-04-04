package query

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
)

// LinksConfig holds parameters for links out/in operations.
type LinksConfig struct {
	Input      string
	Direction  string // "out" or "in"
	EdgeType   string
	JSONOutput bool
	VaultPath  string
}

// RunLinks executes the links out or links in logic.
func RunLinks(db *index.DB, cfg LinksConfig, w io.Writer) error {
	resolver := graph.NewResolver(db)
	resolved, err := resolver.Resolve(cfg.Input)
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}
	if !resolved.Resolved || resolved.Ambiguous {
		if cfg.JSONOutput {
			return json.NewEncoder(w).Encode(
				envelope.Error("links "+cfg.Direction, "resolution_failed",
					fmt.Sprintf("could not resolve %q unambiguously", cfg.Input), ""))
		}
		return fmt.Errorf("could not resolve %q unambiguously", cfg.Input)
	}

	noteID := resolved.Matches[0].ID
	cmdName := "links " + cfg.Direction

	var links []graph.LinkResult
	if cfg.Direction == "out" {
		links, err = graph.LinksOut(db, noteID, cfg.EdgeType)
	} else {
		links, err = graph.LinksIn(db, noteID, cfg.EdgeType)
	}
	if err != nil {
		return fmt.Errorf("querying links: %w", err)
	}

	if cfg.JSONOutput {
		type linksResult struct {
			SourceID string             `json:"source_id,omitempty"`
			TargetID string             `json:"target_id,omitempty"`
			Links    []graph.LinkResult `json:"links"`
		}
		r := linksResult{Links: links}
		if cfg.Direction == "out" {
			r.SourceID = noteID
		} else {
			r.TargetID = noteID
		}
		env := envelope.OK(cmdName, r)
		env.Meta.VaultPath = cfg.VaultPath
		return json.NewEncoder(w).Encode(env)
	}

	for _, l := range links {
		var id string
		if cfg.Direction == "out" {
			if l.TargetID != nil {
				id = *l.TargetID
			} else {
				id = l.TargetRaw
			}
		} else {
			id = l.SourceID
		}
		if _, err := fmt.Fprintf(w, "%-20s %-20s %s\n", id, l.EdgeType, l.Confidence); err != nil {
			return err
		}
	}
	return nil
}
