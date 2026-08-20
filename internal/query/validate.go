package query

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/schema"
)

// Validate rule identifiers — the SSOT for rule strings used across the
// query layer. validate.go is the canonical definition site; doctor.go and any
// other caller must reference these constants instead of inlining the strings.
const (
	RuleBrokenReference = "broken_reference"
	RuleMissingRequired = "missing_required_field"
	RuleUnknownType     = "unknown_type"
	RuleInvalidStatus   = "invalid_status"
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
	// BrokenRefs carries the reference ids that did not resolve, for
	// RuleBrokenReference. It exists so a caller can group by TARGET rather than
	// by source file: eight broken references across two vaults turned out to be
	// one missing note referenced four times plus four unrelated problems, and
	// the per-file view made that look like eight separate things.
	BrokenRefs []string `json:"broken_refs,omitempty"`
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
				Rule: RuleUnknownType, Message: fmt.Sprintf("Type %q not in registry", n.noteType),
				Value: n.noteType,
			})
			noteHasIssue = true
		}

		// Rule: missing_required_field (only for known types)
		if reg.HasType(n.noteType) {
			td, _ := reg.GetTypeDef(n.noteType)
			for _, req := range td.Required {
				val := fieldValue(n, req, db, reg)
				if val == "" {
					result.Issues = append(result.Issues, ValidateIssue{
						Path: n.path, ID: n.id, Severity: "error",
						Rule:    RuleMissingRequired,
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
					Rule:    RuleInvalidStatus,
					Message: fmt.Sprintf("Status %q not valid for type %q", n.status, n.noteType),
					Field:   "status", Value: n.status,
				})
				noteHasIssue = true
			}
		}

		// Rule: broken_reference (check explicit_relation edges)
		brokenRefs, refErr := listBrokenRefs(db, n.id)
		if refErr == nil && len(brokenRefs) > 0 {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: n.path, ID: n.id, Severity: "warning",
				Rule: RuleBrokenReference,
				Message: fmt.Sprintf("frontmatter references do not resolve: %s",
					strings.Join(brokenRefs, ", ")),
				BrokenRefs: brokenRefs,
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

func fieldValue(n noteInfo, field string, db *index.DB, reg *schema.Registry) string {
	// Check dedicated columns first. Aliases on status/title are not yet
	// supported here — the dedicated columns are populated by the indexer
	// from the canonical field name, so an aliased status/title would not
	// reach this point. Status/title aliasing is a follow-on; defer until
	// reality demands it.
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
	// Check frontmatter_kv for the canonical name and any registered
	// aliases. Iterate canonical-first so canonical wins when both are
	// present. The indexer keys frontmatter_kv by the user's actual field
	// name (no normalization), so a vault with `last_updated` only stores
	// `last_updated` — the alias lookup is what makes it satisfy `updated`.
	for _, name := range reg.FieldNamesForLookup(field) {
		var val sql.NullString
		_ = db.QueryRow("SELECT value_json FROM frontmatter_kv WHERE note_id = ? AND key = ?", n.id, name).Scan(&val)
		if val.Valid && val.String != "" && val.String != `""` {
			return val.String
		}
	}
	return ""
}

// listBrokenRefs returns the reference ids that do not resolve — not a count.
//
// It used to SELECT COUNT(*), and the caller rendered "N frontmatter references
// do not resolve": the part the reader already knows (something is wrong)
// without the part they need (which one). The query had every id in hand and
// aggregated them away before printing.
//
// Measured cost: eight broken references across two vaults had to be
// reconstructed by hand against the index before they could be fixed. They were
// five different problems needing five different remedies — one missing note
// referenced four times, one id missing its `source-` prefix, two pointing into
// a different vault, two never written, and one auto-memory filename pasted into
// a vault's related_ids. No count distinguishes those.
func listBrokenRefs(db *index.DB, noteID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT dst_raw FROM links
		WHERE src_note_id = ? AND edge_type = 'explicit_relation'
		AND dst_raw NOT IN (SELECT id FROM notes)
		ORDER BY dst_raw`,
		noteID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}
