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
	Direction  string // "out", "in", or "both"
	EdgeType   string
	JSONOutput bool
	VaultPath  string
	IndexHash  string
}

// OutResult is the JSON-serializable payload for an outbound-links query.
// SourceID is the queried note; Links are the edges leaving it.
type OutResult struct {
	SourceID string          `json:"source_id"`
	Links    []graph.OutLink `json:"links"`
}

// InResult is the JSON-serializable payload for an inbound-links (backlinks)
// query. TargetID is the queried note; Links are the edges pointing at it.
type InResult struct {
	TargetID string         `json:"target_id"`
	Links    []graph.InLink `json:"links"`
}

// BothResult is the combined payload for a `--both` query: it carries the
// outbound and inbound directions in ONE structure so the cmd layer can wrap
// it in a single envelope (out before in) instead of emitting two concatenated
// envelopes (invalid JSON). Inbound links use the same "links" key as the
// standalone --in payload; the object nesting (result.in vs result.out)
// already disambiguates direction.
type BothResult struct {
	Out struct {
		SourceID string          `json:"source_id"`
		Links    []graph.OutLink `json:"links"`
	} `json:"out"`
	In struct {
		TargetID string         `json:"target_id"`
		Links    []graph.InLink `json:"links"`
	} `json:"in"`
}

// resolveNoteID resolves cfg.Input to a single note ID. On a resolution
// failure it returns a non-nil envelope error payload that the caller should
// render (JSON) or surface as a Go error (human) — never both.
func resolveNoteID(db *index.DB, cfg LinksConfig) (string, error) {
	resolver := graph.NewResolver(db)
	resolved, err := resolver.Resolve(cfg.Input)
	if err != nil {
		return "", fmt.Errorf("resolving: %w", err)
	}
	if !resolved.Resolved || resolved.Ambiguous {
		return "", fmt.Errorf("could not resolve %q unambiguously", cfg.Input)
	}
	return resolved.Matches[0].ID, nil
}

// CollectOut returns the outbound-links payload for noteID WITHOUT rendering.
// Split from rendering so the cmd layer can aggregate directions into one
// envelope for the `--both` JSON path.
func CollectOut(db *index.DB, noteID, edgeType string) (OutResult, error) {
	links, err := graph.LinksOut(db, noteID, edgeType)
	if err != nil {
		return OutResult{}, fmt.Errorf("querying links: %w", err)
	}
	return OutResult{SourceID: noteID, Links: links}, nil
}

// CollectIn returns the inbound-links (backlinks) payload for noteID WITHOUT
// rendering.
func CollectIn(db *index.DB, noteID, edgeType string) (InResult, error) {
	links, err := graph.LinksIn(db, noteID, edgeType)
	if err != nil {
		return InResult{}, fmt.Errorf("querying links: %w", err)
	}
	return InResult{TargetID: noteID, Links: links}, nil
}

// CollectBoth resolves cfg.Input and returns the combined out+in payload
// WITHOUT rendering. Used by the cmd layer to build a single `--both` envelope.
func CollectBoth(db *index.DB, cfg LinksConfig) (BothResult, string, error) {
	var both BothResult
	noteID, err := resolveNoteID(db, cfg)
	if err != nil {
		return both, "", err
	}
	out, err := CollectOut(db, noteID, cfg.EdgeType)
	if err != nil {
		return both, noteID, err
	}
	in, err := CollectIn(db, noteID, cfg.EdgeType)
	if err != nil {
		return both, noteID, err
	}
	both.Out.SourceID = out.SourceID
	both.Out.Links = out.Links
	both.In.TargetID = in.TargetID
	both.In.Links = in.Links
	return both, noteID, nil
}

// RunLinks executes a single-direction ("out" or "in") links query and renders
// it to w. The `--both` path is handled by the cmd layer via CollectBoth so it
// can emit ONE envelope rather than two concatenated ones.
func RunLinks(db *index.DB, cfg LinksConfig, w io.Writer) error {
	noteID, err := resolveNoteID(db, cfg)
	if err != nil {
		if cfg.JSONOutput {
			return json.NewEncoder(w).Encode(
				envelope.Error("links "+cfg.Direction, "resolution_failed",
					fmt.Sprintf("could not resolve %q unambiguously", cfg.Input), ""))
		}
		return err
	}

	cmdName := "links " + cfg.Direction
	if cfg.Direction == "out" {
		out, err := CollectOut(db, noteID, cfg.EdgeType)
		if err != nil {
			return err
		}
		return renderOut(out, cfg, cmdName, w)
	}
	in, err := CollectIn(db, noteID, cfg.EdgeType)
	if err != nil {
		return err
	}
	return renderIn(in, cfg, cmdName, w)
}

func renderOut(out OutResult, cfg LinksConfig, cmdName string, w io.Writer) error {
	if cfg.JSONOutput {
		env := envelope.OK(cmdName, out)
		env.Meta.VaultPath = cfg.VaultPath
		env.Meta.IndexHash = cfg.IndexHash
		return json.NewEncoder(w).Encode(env)
	}
	for _, l := range out.Links {
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

func renderIn(in InResult, cfg LinksConfig, cmdName string, w io.Writer) error {
	if cfg.JSONOutput {
		env := envelope.OK(cmdName, in)
		env.Meta.VaultPath = cfg.VaultPath
		env.Meta.IndexHash = cfg.IndexHash
		return json.NewEncoder(w).Encode(env)
	}
	for _, l := range in.Links {
		if _, err := fmt.Fprintf(w, "%-20s %-20s %s\n", l.SourceID, l.EdgeType, l.Confidence); err != nil {
			return err
		}
	}
	return nil
}
