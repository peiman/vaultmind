// Package query provides read-only vault diagnostics and search operations.
package query

import (
	"fmt"

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
	DuplicateIDs          int `json:"duplicate_ids"`
	BrokenReferences      int `json:"broken_references"`
	MissingRequiredFields int `json:"missing_required_fields"`
	MalformedMarkers      int `json:"malformed_markers"`
	UnresolvedLinks       int `json:"unresolved_links"`
	NotesMissingIDOrType  int `json:"notes_missing_id_or_type"`
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

	// Duplicate IDs (should be 0 with UNIQUE constraint, but check anyway)
	if err := db.QueryRow("SELECT COUNT(*) FROM (SELECT id FROM notes GROUP BY id HAVING COUNT(*) > 1)").Scan(&result.Issues.DuplicateIDs); err != nil {
		return nil, fmt.Errorf("counting duplicate IDs: %w", err)
	}

	return result, nil
}
