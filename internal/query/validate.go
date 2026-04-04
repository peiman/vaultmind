package query

import (
	"database/sql"
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
)

// ValidateResult is the JSON-serializable output of frontmatter validate.
type ValidateResult struct {
	FilesChecked int             `json:"files_checked"`
	Valid        int             `json:"valid"`
	Issues       []ValidateIssue `json:"issues"`
}

// ValidateIssue represents a single validation finding.
type ValidateIssue struct {
	Path     string `json:"path"`
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Value    string `json:"value,omitempty"`
}

// Validate runs all frontmatter validation rules against the indexed notes.
func Validate(db *index.DB, reg *schema.Registry) (*ValidateResult, error) {
	result := &ValidateResult{
		Issues: []ValidateIssue{},
	}

	rows, err := db.Query("SELECT id, path, type, status, title, is_domain FROM notes")
	if err != nil {
		return nil, fmt.Errorf("querying notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notes []noteInfo
	for rows.Next() {
		var n noteInfo
		var t, s, title sql.NullString
		if scanErr := rows.Scan(&n.id, &n.path, &t, &s, &title, &n.isDomain); scanErr != nil {
			return nil, fmt.Errorf("scanning note: %w", scanErr)
		}
		n.noteType = t.String
		n.status = s.String
		n.title = title.String
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result.FilesChecked = len(notes)

	for _, n := range notes {
		if !n.isDomain {
			result.Valid++
			continue
		}

		noteHasIssue := false

		// Rule: unknown_type
		if n.noteType != "" && !reg.HasType(n.noteType) {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: n.path, ID: n.id, Severity: "warning",
				Rule: "unknown_type", Message: fmt.Sprintf("Type %q not in registry", n.noteType),
				Value: n.noteType,
			})
			noteHasIssue = true
		}

		// Rule: missing_required_field (only for known types)
		if reg.HasType(n.noteType) {
			td, _ := reg.GetTypeDef(n.noteType)
			for _, req := range td.Required {
				val := fieldValue(n, req, db)
				if val == "" {
					result.Issues = append(result.Issues, ValidateIssue{
						Path: n.path, ID: n.id, Severity: "error",
						Rule:    "missing_required_field",
						Message: fmt.Sprintf("Type %q requires field %q", n.noteType, req),
						Field:   req,
					})
					noteHasIssue = true
				}
			}

			// Rule: invalid_status
			if n.status != "" && len(td.Statuses) > 0 && !reg.ValidStatus(n.noteType, n.status) {
				result.Issues = append(result.Issues, ValidateIssue{
					Path: n.path, ID: n.id, Severity: "warning",
					Rule:    "invalid_status",
					Message: fmt.Sprintf("Status %q not valid for type %q", n.status, n.noteType),
					Field:   "status", Value: n.status,
				})
				noteHasIssue = true
			}
		}

		// Rule: broken_reference (check explicit_relation edges)
		brokenRefs, refErr := countBrokenRefs(db, n.id)
		if refErr == nil && brokenRefs > 0 {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: n.path, ID: n.id, Severity: "warning",
				Rule:    "broken_reference",
				Message: fmt.Sprintf("%d frontmatter references do not resolve to existing notes", brokenRefs),
			})
			noteHasIssue = true
		}

		if !noteHasIssue {
			result.Valid++
		}
	}

	return result, nil
}

type noteInfo struct {
	id       string
	path     string
	noteType string
	status   string
	title    string
	isDomain bool
}

func fieldValue(n noteInfo, field string, db *index.DB) string {
	// Check dedicated columns first
	switch field {
	case "status":
		if n.status != "" {
			return n.status
		}
	case "title":
		if n.title != "" {
			return n.title
		}
	}
	// Check frontmatter_kv for domain-tier fields (e.g., url, owner_id)
	var val sql.NullString
	_ = db.QueryRow("SELECT value_json FROM frontmatter_kv WHERE note_id = ? AND key = ?", n.id, field).Scan(&val)
	if val.Valid && val.String != "" && val.String != `""` {
		return val.String
	}
	return ""
}

func countBrokenRefs(db *index.DB, noteID string) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM links
		WHERE src_note_id = ? AND edge_type = 'explicit_relation'
		AND dst_raw NOT IN (SELECT id FROM notes)`,
		noteID,
	).Scan(&count)
	return count, err
}
