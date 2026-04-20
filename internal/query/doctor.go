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
	VaultPath         string            `json:"vault_path"`
	TotalFiles        int               `json:"total_files"`
	DomainNotes       int               `json:"domain_notes"`
	UnstructuredNotes int               `json:"unstructured_notes"`
	IndexStatus       string            `json:"index_status"`
	Embeddings        *DoctorEmbeddings `json:"embeddings,omitempty"`
	Issues            DoctorIssues      `json:"issues"`
}

// DoctorEmbeddings reports the vault's semantic-retrieval readiness. Surfaces
// which embedding lanes are populated so a user can diagnose a keyword-only
// fallback at a glance without running an ask query and hitting zero hits.
type DoctorEmbeddings struct {
	TotalNotes    int    `json:"total_notes"`
	DenseCount    int    `json:"dense_count"`
	SparseCount   int    `json:"sparse_count"`
	ColBERTCount  int    `json:"colbert_count"`
	Model         string `json:"model"` // "bge-m3", "minilm", or "" when no dense embeddings
	SemanticReady bool   `json:"semantic_ready"`
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
	PathPseudoIDLinks         int                `json:"path_pseudo_id_links"`
	PathPseudoIDDetails       []UnresolvedLink   `json:"path_pseudo_id_details,omitempty"`
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

	// Links resolved to _path: pseudo-IDs (files that don't exist in the vault)
	pseudoRows, err := db.Query(`
		SELECT l.src_note_id, COALESCE(sn.path, ''), l.dst_raw
		FROM links l
		LEFT JOIN notes sn ON sn.id = l.src_note_id
		WHERE l.resolved = TRUE AND SUBSTR(l.dst_note_id, 1, 6) = '_path:'`)
	if err != nil {
		return nil, fmt.Errorf("querying pseudo-ID links: %w", err)
	}
	defer func() { _ = pseudoRows.Close() }()

	result.Issues.PathPseudoIDDetails = []UnresolvedLink{}
	for pseudoRows.Next() {
		var pl UnresolvedLink
		if scanErr := pseudoRows.Scan(&pl.SourceID, &pl.SourcePath, &pl.TargetRaw); scanErr != nil {
			return nil, fmt.Errorf("scanning pseudo-ID link: %w", scanErr)
		}
		result.Issues.PathPseudoIDDetails = append(result.Issues.PathPseudoIDDetails, pl)
	}
	if err := pseudoRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pseudo-ID links: %w", err)
	}
	result.Issues.PathPseudoIDLinks = len(result.Issues.PathPseudoIDDetails)

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

	emb, err := collectEmbeddingStatus(db, result.TotalFiles)
	if err != nil {
		return nil, err
	}
	result.Embeddings = emb

	return result, nil
}

// collectEmbeddingStatus inspects the index DB for per-lane embedding counts
// and infers the model from dense-embedding dimensionality (384=minilm,
// 1024=bge-m3). SemanticReady is driven by dense presence since it's the
// required lane for ask's auto-retriever to engage hybrid mode.
func collectEmbeddingStatus(db *index.DB, total int) (*DoctorEmbeddings, error) {
	emb := &DoctorEmbeddings{TotalNotes: total}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL").Scan(&emb.DenseCount); err != nil {
		return nil, fmt.Errorf("counting dense embeddings: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE sparse_embedding IS NOT NULL").Scan(&emb.SparseCount); err != nil {
		return nil, fmt.Errorf("counting sparse embeddings: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE colbert_embedding IS NOT NULL").Scan(&emb.ColBERTCount); err != nil {
		return nil, fmt.Errorf("counting colbert embeddings: %w", err)
	}
	emb.SemanticReady = emb.DenseCount > 0
	if emb.DenseCount > 0 {
		dims, err := index.DetectEmbeddingDims(db)
		if err != nil {
			return nil, fmt.Errorf("detecting embedding dims: %w", err)
		}
		switch dims {
		case 384:
			emb.Model = "minilm"
		case 1024:
			emb.Model = "bge-m3"
		}
	}
	return emb, nil
}
