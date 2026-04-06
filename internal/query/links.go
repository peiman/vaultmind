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
	IndexHash  string
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

	if cfg.Direction == "out" {
		return runLinksOut(db, noteID, cfg, cmdName, w)
	}
	return runLinksIn(db, noteID, cfg, cmdName, w)
}

func runLinksOut(db *index.DB, noteID string, cfg LinksConfig, cmdName string, w io.Writer) error {
	links, err := graph.LinksOut(db, noteID, cfg.EdgeType)
	if err != nil {
		return fmt.Errorf("querying links: %w", err)
	}

	if cfg.JSONOutput {
		type outResult struct {
			SourceID string          `json:"source_id"`
			Links    []graph.OutLink `json:"links"`
		}
		env := envelope.OK(cmdName, outResult{SourceID: noteID, Links: links})
		env.Meta.VaultPath = cfg.VaultPath
		env.Meta.IndexHash = cfg.IndexHash
		return json.NewEncoder(w).Encode(env)
	}

	for _, l := range links {
		id := l.TargetRaw
		if l.TargetID != nil {
			id = *l.TargetID
		}
		if _, err := fmt.Fprintf(w, "%-20s %-20s %s\n", id, l.EdgeType, l.Confidence); err != nil {
			return err
		}
	}
	return nil
}

func runLinksIn(db *index.DB, noteID string, cfg LinksConfig, cmdName string, w io.Writer) error {
	links, err := graph.LinksIn(db, noteID, cfg.EdgeType)
	if err != nil {
		return fmt.Errorf("querying links: %w", err)
	}

	if cfg.JSONOutput {
		type inResult struct {
			TargetID string         `json:"target_id"`
			Links    []graph.InLink `json:"links"`
		}
		env := envelope.OK(cmdName, inResult{TargetID: noteID, Links: links})
		env.Meta.VaultPath = cfg.VaultPath
		env.Meta.IndexHash = cfg.IndexHash
		return json.NewEncoder(w).Encode(env)
	}

	for _, l := range links {
		if _, err := fmt.Fprintf(w, "%-20s %-20s %s\n", l.SourceID, l.EdgeType, l.Confidence); err != nil {
			return err
		}
	}
	return nil
}
