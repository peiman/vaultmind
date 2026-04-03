package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

func runLinksDirection(cmd *cobra.Command, input, direction string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppLinksVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppLinksJson)
	edgeType := getConfigValueWithFlags[string](cmd, "edge-type", config.KeyAppLinksEdgeType)

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	db, err := index.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer func() { _ = db.Close() }()

	resolver := graph.NewResolver(db)
	resolved, err := resolver.Resolve(input)
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}
	if !resolved.Resolved || resolved.Ambiguous {
		return fmt.Errorf("could not resolve %q unambiguously", input)
	}
	noteID := resolved.Matches[0].ID

	var links []graph.LinkResult
	cmdName := "links " + direction
	if direction == "out" {
		links, err = graph.LinksOut(db, noteID, edgeType)
	} else {
		links, err = graph.LinksIn(db, noteID, edgeType)
	}
	if err != nil {
		return fmt.Errorf("querying links: %w", err)
	}

	if jsonOut {
		type linksResult struct {
			SourceID string             `json:"source_id,omitempty"`
			TargetID string             `json:"target_id,omitempty"`
			Links    []graph.LinkResult `json:"links"`
		}
		r := linksResult{Links: links}
		if direction == "out" {
			r.SourceID = noteID
		} else {
			r.TargetID = noteID
		}
		env := envelope.OK(cmdName, r)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	for _, l := range links {
		var id string
		if direction == "out" {
			if l.TargetID != nil {
				id = *l.TargetID
			} else {
				id = l.TargetRaw
			}
		} else {
			id = l.SourceID
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %s\n", id, l.EdgeType, l.Confidence); err != nil {
			return err
		}
	}
	return nil
}
