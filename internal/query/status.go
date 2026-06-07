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
	}

	// Note counts
	if err := db.QueryRow("SELECT COUNT(*) FROM notes").Scan(&result.TotalFiles); err != nil {
		return nil, fmt.Errorf("counting notes: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE is_domain = TRUE").Scan(&result.DomainNotes); err != nil {
		return nil, fmt.Errorf("counting domain notes: %w", err)
	}
	result.UnstructuredNotes = result.TotalFiles - result.DomainNotes

	// Per-type breakdown and issues rollup come from the shared helpers so the
	// cold-start view here and the doctor health hub stay in lockstep (SSOT).
	types, err := CollectTypeBreakdown(db, cfg)
	if err != nil {
		return nil, err
	}
	result.Types = types

	summary, err := SummarizeValidationIssues(db, reg)
	if err != nil {
		return nil, err
	}
	result.IssuesSummary = summary

	return result, nil
}

// CollectTypeBreakdown returns the per-type note counts together with each
// type's required fields and valid statuses, drawn from the vault config's
// type registry. It is the single source of truth for the per-type breakdown
// surfaced by both `vault status` (cold-start) and `doctor` (health hub).
// A nil cfg yields an empty (non-nil) map so callers can range safely.
func CollectTypeBreakdown(db *index.DB, cfg *vault.Config) (map[string]StatusTypeInfo, error) {
	types := make(map[string]StatusTypeInfo)
	if cfg == nil {
		return types, nil
	}
	for name, td := range cfg.Types {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE type = ?", name).Scan(&count); err != nil {
			return nil, fmt.Errorf("counting type %q: %w", name, err)
		}
		statuses := td.Statuses
		if statuses == nil {
			statuses = []string{}
		}
		types[name] = StatusTypeInfo{
			Count:    count,
			Required: td.Required,
			Statuses: statuses,
		}
	}
	return types, nil
}

// SummarizeValidationIssues runs the schema validator and rolls its issues up
// into error/warning counts. Shared by `vault status` and `doctor` so both
// report the same rollup (SSOT). A nil reg yields a zero-value summary.
func SummarizeValidationIssues(db *index.DB, reg *schema.Registry) (StatusIssuesSummary, error) {
	var summary StatusIssuesSummary
	if reg == nil {
		return summary, nil
	}
	valResult, err := Validate(db, reg)
	if err != nil {
		return summary, fmt.Errorf("running validation: %w", err)
	}
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			summary.Errors++
		} else {
			summary.Warnings++
		}
	}
	return summary, nil
}
