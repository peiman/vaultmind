// Package query provides read-only vault diagnostics and search operations.
package query

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
)

// DoctorResult is the JSON-serializable output of the doctor command.
type DoctorResult struct {
	VaultPath         string       `json:"vault_path"`
	TotalFiles        int          `json:"total_files"`
	DomainNotes       int          `json:"domain_notes"`
	UnstructuredNotes int          `json:"unstructured_notes"`
	IndexStatus       string       `json:"index_status"`
	Issues            DoctorIssues `json:"issues"`
}

// DoctorIssues holds counts of vault health issues.
type DoctorIssues struct {
	DuplicateIDs              int                `json:"duplicate_ids"`
	BrokenReferences          int                `json:"broken_references"`
	MissingRequiredFields     int                `json:"missing_required_fields"`
	MalformedMarkers          int                `json:"malformed_markers"`
	UnresolvedLinks           int                `json:"unresolved_links"`
	NotesMissingIDOrType      int                `json:"notes_missing_id_or_type"`
	UnresolvedLinkDetails     []UnresolvedLink   `json:"unresolved_link_details,omitempty"`
	ObsidianIncompatibleLinks int                `json:"obsidian_incompatible_links"`
	IncompatibleLinkDetails   []IncompatibleLink `json:"incompatible_link_details,omitempty"`
}

// UnresolvedLink describes a single unresolved link with source and target info.
type UnresolvedLink struct {
	SourceID   string `json:"source_id"`
	SourcePath string `json:"source_path"`
	TargetRaw  string `json:"target_raw"`
}

// IncompatibleLink describes a wikilink that resolves in VaultMind but not in Obsidian.
// Obsidian resolves [[target]] by matching target against filenames (without extension).
// If dst_raw uses a note's title instead of its filename stem, Obsidian won't find it.
type IncompatibleLink struct {
	SourceID     string `json:"source_id"`
	SourcePath   string `json:"source_path"`
	TargetRaw    string `json:"target_raw"`
	SuggestedFix string `json:"suggested_fix"`
}

// Doctor runs vault health diagnostics against the indexed database.
func Doctor(db *index.DB, vaultPath string) (*DoctorResult, error) {
	result := &DoctorResult{
		VaultPath:   vaultPath,
		IndexStatus: "current",
	}

	// Count total, domain, unstructured
	if err := db.QueryRow("SELECT COUNT(*) FROM notes").Scan(&result.TotalFiles); err != nil {
		return nil, fmt.Errorf("counting notes: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE is_domain = TRUE").Scan(&result.DomainNotes); err != nil {
		return nil, fmt.Errorf("counting domain notes: %w", err)
	}
	result.UnstructuredNotes = result.TotalFiles - result.DomainNotes

	// Unresolved links
	if err := db.QueryRow("SELECT COUNT(*) FROM links WHERE resolved = FALSE AND dst_note_id IS NULL").Scan(&result.Issues.UnresolvedLinks); err != nil {
		return nil, fmt.Errorf("counting unresolved links: %w", err)
	}

	// Unresolved link details
	if result.Issues.UnresolvedLinks > 0 {
		detailRows, err := db.Query(
			`SELECT l.src_note_id, COALESCE(n.path, ''), l.dst_raw
			 FROM links l
			 LEFT JOIN notes n ON n.id = l.src_note_id
			 WHERE l.resolved = FALSE AND l.dst_note_id IS NULL`)
		if err != nil {
			return nil, fmt.Errorf("querying unresolved link details: %w", err)
		}
		defer func() { _ = detailRows.Close() }()

		result.Issues.UnresolvedLinkDetails = []UnresolvedLink{}
		for detailRows.Next() {
			var ul UnresolvedLink
			if scanErr := detailRows.Scan(&ul.SourceID, &ul.SourcePath, &ul.TargetRaw); scanErr != nil {
				return nil, fmt.Errorf("scanning unresolved link detail: %w", scanErr)
			}
			result.Issues.UnresolvedLinkDetails = append(result.Issues.UnresolvedLinkDetails, ul)
		}
		if err := detailRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating unresolved link details: %w", err)
		}
	}

	// Duplicate IDs (should be 0 with UNIQUE constraint, but check anyway)
	if err := db.QueryRow("SELECT COUNT(*) FROM (SELECT id FROM notes GROUP BY id HAVING COUNT(*) > 1)").Scan(&result.Issues.DuplicateIDs); err != nil {
		return nil, fmt.Errorf("counting duplicate IDs: %w", err)
	}

	// Obsidian-incompatible links: resolved wikilinks where dst_raw doesn't
	// match the target note's filename stem. Obsidian resolves [[X]] by matching
	// X against filenames (without extension and directory), not note titles.
	incompatRows, err := db.Query(`
		SELECT l.src_note_id, COALESCE(sn.path, ''), l.dst_raw, n.path
		FROM links l
		JOIN notes n ON n.id = l.dst_note_id
		LEFT JOIN notes sn ON sn.id = l.src_note_id
		WHERE l.resolved = TRUE
		AND l.edge_type IN ('explicit_link', 'explicit_embed')`)
	if err != nil {
		return nil, fmt.Errorf("querying incompatible links: %w", err)
	}
	defer func() { _ = incompatRows.Close() }()

	result.Issues.IncompatibleLinkDetails = []IncompatibleLink{}
	seen := make(map[string]bool)
	for incompatRows.Next() {
		var srcID, srcPath, dstRaw, dstPath string
		if scanErr := incompatRows.Scan(&srcID, &srcPath, &dstRaw, &dstPath); scanErr != nil {
			return nil, fmt.Errorf("scanning incompatible link: %w", scanErr)
		}
		filenameStem := strings.TrimSuffix(filepath.Base(dstPath), ".md")
		if dstRaw == filenameStem {
			continue // already compatible
		}
		// Also skip if dst_raw contains "|" (already uses [[file|display]] format)
		if strings.Contains(dstRaw, "|") {
			continue
		}
		key := srcID + "\x00" + dstRaw
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Issues.IncompatibleLinkDetails = append(result.Issues.IncompatibleLinkDetails, IncompatibleLink{
			SourceID:     srcID,
			SourcePath:   srcPath,
			TargetRaw:    dstRaw,
			SuggestedFix: filenameStem,
		})
	}
	if err := incompatRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating incompatible links: %w", err)
	}
	result.Issues.ObsidianIncompatibleLinks = len(result.Issues.IncompatibleLinkDetails)

	return result, nil
}
