package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// StatusTypeInfo describes a note type with its count and schema.
type StatusTypeInfo struct {
	Count    int      `json:"count"`
	Required []string `json:"required"`
	Statuses []string `json:"statuses"`
}

// StatusIssuesSummary holds aggregated issue counts.
type StatusIssuesSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// StatusResult is the JSON response for vault status.
type StatusResult struct {
	VaultPath         string                    `json:"vault_path"`
	TotalFiles        int                       `json:"total_files"`
	DomainNotes       int                       `json:"domain_notes"`
	UnstructuredNotes int                       `json:"unstructured_notes"`
	IndexStatus       string                    `json:"index_status"`
	IndexStale        bool                      `json:"index_stale"`
	Types             map[string]StatusTypeInfo `json:"types"`
	IssuesSummary     StatusIssuesSummary       `json:"issues_summary"`
}

// VaultStatus combines doctor, schema, and validation into a single cold-start response.
func VaultStatus(db *index.DB, vaultPath string, cfg *vault.Config, reg *schema.Registry) (*StatusResult, error) {
	result := &StatusResult{
		VaultPath:   vaultPath,
		IndexStatus: "current",
		Types:       make(map[string]StatusTypeInfo),
	}

	// Note counts
	if err := db.QueryRow("SELECT COUNT(*) FROM notes").Scan(&result.TotalFiles); err != nil {
		return nil, fmt.Errorf("counting notes: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE is_domain = TRUE").Scan(&result.DomainNotes); err != nil {
		return nil, fmt.Errorf("counting domain notes: %w", err)
	}
	result.UnstructuredNotes = result.TotalFiles - result.DomainNotes

	// Type info with counts
	for name, td := range cfg.Types {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE type = ?", name).Scan(&count); err != nil {
			return nil, fmt.Errorf("counting type %q: %w", name, err)
		}
		statuses := td.Statuses
		if statuses == nil {
			statuses = []string{}
		}
		result.Types[name] = StatusTypeInfo{
			Count:    count,
			Required: td.Required,
			Statuses: statuses,
		}
	}

	// Issues summary from validation
	valResult, err := Validate(db, reg)
	if err != nil {
		return nil, fmt.Errorf("running validation: %w", err)
	}
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			result.IssuesSummary.Errors++
		} else {
			result.IssuesSummary.Warnings++
		}
	}

	return result, nil
}
